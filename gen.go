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
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/pointer"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types"
)

type Gen struct {
	loader.Config
	Prog *loader.Program

	// A function which returns an io.WriteCloser for given file path to be rewritten. Can return nil for non-target files.
	FileWriter func(string) io.WriteCloser

	Verbose bool

	// Main specifies main package for pointer analysis.
	// If not set, the ad-hoc package created by CreateFromFilenames is used.
	Main string

	ssaProg *ssa.Program
}

func New() *Gen {
	g := &Gen{}
	g.SourceImports = true
	g.ParserMode = parser.ParseComments
	return g
}

func (g *Gen) RewriteFiles() error {
	err := g.initProg()
	if err != nil {
		return err
	}

	return g.rewriteProg()
}

func (g *Gen) initProg() (err error) {
	g.Prog, err = g.Load()
	if err != nil {
		return err
	}

	mode := ssa.SanityCheckFunctions
	g.ssaProg = ssa.Create(g.Prog, mode)
	g.ssaProg.BuildAll()

	return nil
}

func (g *Gen) writeNode(w io.WriteCloser, node interface{}) error {
	err := format.Node(w, g.Fset, node)
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

	pkg, path, _ := g.Prog.PathEnclosingInterval(funcDecl.Pos(), funcDecl.End())
	ssaPkg := g.ssaProg.Package(pkg.Pkg)

	ssaFn := ssa.EnclosingFunction(ssaPkg, path)
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
	// pkg, _, _ := g.Prog.PathEnclosingInterval(file.Pos(), file.End())
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

			typeSwitch := NewTypeSwitchStmt(g, file, sw, pkg.Info)
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

func (g *Gen) pointerAnalysis() (*pointer.Result, error) {
	// Either an ad-hoc package is created
	// or the package specified by g.Main is loaded
	var pkg *loader.PackageInfo
	if len(g.Prog.Created) > 0 {
		pkg = g.Prog.Created[0]
	} else {
		pkg = g.Prog.Imported[g.Main]
	}

	if pkg == nil {
		return nil, fmt.Errorf("BUG: no package is created and main %q is not imported")
	}

	ssaPkg := g.ssaProg.Package(pkg.Pkg)

	var ssaMain *ssa.Package
	if _, ok := ssaPkg.Members["main"]; ok {
		ssaMain = ssaPkg
	} else {
		ssaTestPkg := g.ssaProg.CreateTestMainPackage(ssaPkg)
		if ssaTestPkg == nil {
			return nil, fmt.Errorf("%s does not have main function nor tests", pkg)
		}

		ssaMain = ssaTestPkg
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
	for _, pkg := range g.Prog.AllPackages {
		for _, file := range pkg.Files {
			w := g.FileWriter(filepath.Clean(g.Fset.File(file.Pos()).Name()))
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

func (g *Gen) log(file *ast.File, node ast.Node, pattern string, args ...interface{}) {
	if g.Verbose == false {
		return
	}

	pos := g.Fset.File(file.Pos()).Position(node.Pos())

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
	format.Node(&buf, g.Fset, node)
	return buf.String()
}
