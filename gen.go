package gen

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/pointer"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types"
)

// Gen is the typeswitch-gen API object.
type Gen struct {
	// Users can use Loader as a configuration; Its resulting program is used by Gen
	Loader loader.Config

	// A function which returns an io.WriteCloser for given file path to be rewritten. Can return nil for non-target files.
	FileWriter func(string) io.WriteCloser

	// Main specifies main package for pointer analysis.
	// If not set, the ad-hoc package created by CreateFromFilenames is used.
	Main string

	Verbose bool

	program    *loader.Program
	ssaProgram *ssa.Program
}

// New creates a Gen with some initial configuration.
func New() *Gen {
	g := &Gen{}
	g.Loader.SourceImports = true
	g.Loader.ParserMode = parser.ParseComments
	return g
}

// RewriteFiles does the AST rewriting using g.Loader.
// It calls g.FileWriter with file paths.
func (g *Gen) RewriteFiles() error {
	err := g.initProg()
	if err != nil {
		return err
	}

	return g.rewriteProg()
}

// initProg loads the program and does SSA analysis.
func (g *Gen) initProg() (err error) {
	g.program, err = g.Loader.Load()
	if err != nil {
		return err
	}

	mode := ssa.SanityCheckFunctions
	g.ssaProgram = ssa.Create(g.program, mode)
	g.ssaProgram.BuildAll()

	return nil
}

func (g *Gen) writeNode(w io.WriteCloser, node interface{}) error {
	err := format.Node(w, g.Loader.Fset, node)
	if err != nil {
		return err
	}

	return w.Close()
}

func (g *Gen) callGraphInEdges(funcDecl *ast.FuncDecl) ([]*callgraph.Edge, error) {
	pta, err := g.pointerAnalysis()
	if err != nil {
		return nil, err
	}

	pkg, path, _ := g.program.PathEnclosingInterval(funcDecl.Pos(), funcDecl.End())
	ssaFn := ssa.EnclosingFunction(g.ssaPackage(pkg), path)
	if ssaFn == nil {
		return nil, fmt.Errorf("BUG: could not find SSA function: %s", funcDecl.Name)
	}

	return pta.CallGraph.CreateNode(ssaFn).In, nil
}

func namedParamPos(name string, list *ast.FieldList) int {
	var pos int
	for _, f := range list.List {
		for _, n := range f.Names {
			if n.Name == name {
				return pos
			}
			pos = pos + 1
		}
	}

	return -1
}

func argTypesAt(pos int, edges []*callgraph.Edge) []types.Type {
	inTypes := []types.Type{}

	for _, edge := range edges {
		site := edge.Site
		if site == nil {
			continue
		}

		a := site.Common().Args[pos]
		if mi, ok := a.(*ssa.MakeInterface); ok {
			inTypes = append(inTypes, mi.X.Type())
		}
	}

	return inTypes
}

// rewriteFile is the main logic. May rewrite type switch statements in ast.File file.
func (g *Gen) rewriteFile(pkg *loader.PackageInfo, file *ast.File) error {
	// XXX We can also obtain *loader.PackageInfo by:
	// pkg, _, _ := g.program.PathEnclosingInterval(file.Pos(), file.End())
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if funcDecl.Body == nil {
			// Maybe an object-provided function and we have no source information
			continue
		}

		// For each type switch statements...
		for _, stmt := range funcDecl.Body.List {
			sw, ok := stmt.(*ast.TypeSwitchStmt)
			if !ok {
				continue
			}

			g.log(file, sw, "type switch statement: %s", sw.Assign)

			typeSwitch := newTypeSwitchStmt(g, file, sw, pkg.Info)
			if typeSwitch == nil {
				continue
			}

			target := typeSwitch.Target()

			g.log(file, funcDecl, "enclosing func: %s", funcDecl.Type)

			// TODO check target is an interface{}

			// XXX parentScope must be of a func
			// scope := pkg.Scopes[sw]
			// parentScope, _ := scope.LookupParent(target.Name)
			// assert(pkg.Scopes[funcDecl.Type] == parentScope)

			// argument index of the variable which is target of the type switch
			in, err := g.callGraphInEdges(funcDecl)
			if err != nil {
				return err
			}

			paramPos := namedParamPos(target.Name, funcDecl.Type.Params)
			inTypes := argTypesAt(paramPos, in)
			for _, inType := range inTypes {
				g.log(file, funcDecl, "argument type: %s (from %s)", inType, in[0].Caller.Func)
			}

			// Finally rewrite it
			*sw = *typeSwitch.Expand(inTypes)
		}
	}

	return nil
}

func (g *Gen) mainPkg() (*loader.PackageInfo, error) {
	// Either an ad-hoc package is created
	// or the package specified by g.Main is loaded
	var pkg *loader.PackageInfo
	if len(g.program.Created) > 0 {
		pkg = g.program.Created[0]
	} else {
		pkg = g.program.Imported[g.Main]
	}

	if pkg == nil {
		return nil, fmt.Errorf("BUG: no package is created and main %q is not imported")
	}

	return pkg, nil
}

func (g *Gen) ssaPackage(pkg *loader.PackageInfo) *ssa.Package {
	return g.ssaProgram.Package(pkg.Pkg)
}

func (g *Gen) pointerAnalysis() (*pointer.Result, error) {
	pkg, err := g.mainPkg()
	if err != nil {
		return nil, err
	}
	ssaPkg := g.ssaPackage(pkg)

	var ssaMain *ssa.Package
	if _, ok := ssaPkg.Members["main"]; ok {
		ssaMain = ssaPkg
	} else {
		ssaMain = g.ssaProgram.CreateTestMainPackage(ssaPkg)
		if ssaMain == nil {
			return nil, fmt.Errorf("%s does not have main function nor tests", pkg)
		}
	}

	conf := &pointer.Config{
		BuildCallGraph: true,
		Mains:          []*ssa.Package{ssaMain},
	}

	return pointer.Analyze(conf)
}

// rewriteProg rewrites each files of each packages loaded
// Must be called after initProg.
func (g *Gen) rewriteProg() (err error) {
	for _, pkg := range g.program.AllPackages {
		for _, file := range pkg.Files {
			w := g.FileWriter(filepath.Clean(g.tokenFile(file).Name()))
			if w == nil {
				continue
			}

			err = g.rewriteFile(pkg, file)
			if err != nil {
				return
			}

			err = g.writeNode(w, file)
			if err != nil {
				return
			}
		}
	}

	return
}

func (g *Gen) tokenFile(node ast.Node) *token.File {
	return g.Loader.Fset.File(node.Pos())
}

func (g *Gen) log(file *ast.File, node ast.Node, pattern string, args ...interface{}) {
	if g.Verbose == false {
		return
	}

	pos := g.tokenFile(file).Position(node.Pos())

	for i, a := range args {
		if node, ok := a.(ast.Node); ok {
			args[i] = g.showNode(node)
		}
	}

	args = append([]interface{}{pos}, args...)
	fmt.Fprintf(os.Stderr, "%s: "+pattern+"\n", args...)
}

func (g *Gen) showNode(node ast.Node) string {
	var buf bytes.Buffer
	format.Node(&buf, g.Loader.Fset, node)
	return buf.String()
}
