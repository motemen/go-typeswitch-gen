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
	// Users can use Loader as a configuration; Its resulting program is used by Gen.
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
	g.Loader.ParserMode = parser.ParseComments
	return g
}

// Expand expands type switches in the program with their template case clauses
// and actual arguments.
func (g Gen) Expand() error {
	err := g.buildSSA()
	if err != nil {
		return err
	}

	return g.doFiles(g.expandFileTypeSwitches)
}

// Sort sorts case clauses in the type switches in the program.
func (g Gen) Sort() error {
	err := g.load()
	if err != nil {
		return err
	}

	return g.doFiles(g.sortFileTypeSwitches)
}

// Scaffold fills type switches with empty case clauses using their subjects type.
func (g Gen) Scaffold() error {
	err := g.load()
	if err != nil {
		return err
	}

	return g.doFiles(g.scaffoldFileTypeSwitches)
}

// load loads the program.
func (g *Gen) load() (err error) {
	g.program, err = g.Loader.Load()
	return
}

// buildSSA loads the program and does SSA analysis.
func (g *Gen) buildSSA() error {
	err := g.load()
	if err != nil {
		return err
	}

	mode := ssa.SanityCheckFunctions
	g.ssaProgram = ssa.NewProgram(g.program.Fset, mode) // FIXME not sure
	g.ssaProgram.BuildAll()

	return nil
}

func (g Gen) writeNode(w io.WriteCloser, node interface{}) error {
	err := format.Node(w, g.Loader.Fset, node)
	if err != nil {
		return err
	}

	return w.Close()
}

func (g Gen) callGraphInEdges(funcDecl *ast.FuncDecl) ([]*callgraph.Edge, error) {
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

func argTypesAt(nth int, edges []*callgraph.Edge) []types.Type {
	inTypes := []types.Type{}

	for _, edge := range edges {
		site := edge.Site
		if site == nil {
			continue
		}

		a := site.Common().Args[nth]
		if mi, ok := a.(*ssa.MakeInterface); ok {
			inTypes = append(inTypes, mi.X.Type())
		}
	}

	return inTypes
}

func (g Gen) possibleSubjectTypes(pkg *loader.PackageInfo, funcDecl *ast.FuncDecl, typeSwitch *typeSwitchStmt) ([]types.Type, error) {
	// XXX We can also obtain *loader.PackageInfo by:
	// pkg, _, _ := g.program.PathEnclosingInterval(file.Pos(), file.End())

	subject := typeSwitch.subject()
	subjectObj := pkg.Info.Uses[subject] // Where the type switch statement subject is defined
	// g.log(file, funcDecl, "enclosing func: %s", funcDecl.Type)
	if subjectObj.Parent() != pkg.Scopes[funcDecl.Type] {
		return nil, fmt.Errorf("BUG: scope mismatch")
	}

	paramPos := namedParamPos(subject.Name, funcDecl.Type.Params)

	// argument index of the variable which is subject of the type switch
	in, err := g.callGraphInEdges(funcDecl)
	if err != nil {
		return nil, err
	}

	return argTypesAt(paramPos, in), nil
}

func (g Gen) mainPkg() (*loader.PackageInfo, error) {
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

func (g Gen) ssaPackage(pkg *loader.PackageInfo) *ssa.Package {
	return g.ssaProgram.Package(pkg.Pkg)
}

func (g Gen) pointerAnalysis() (*pointer.Result, error) {
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

// doFiles is a utility method which calls rewrite for each *ast.File file in the program loaded
// and writes out the modified file (to stdout or the original file).
// rewrite is expected to modify the *ast.File file given.
// It uses g.FileWriter to determine if the file is in target or not.
// Must be called after g.load().
func (g Gen) doFiles(rewrite func(*loader.PackageInfo, *ast.File) error) (err error) {
	for _, pkg := range g.program.AllPackages {
		for _, file := range pkg.Files {
			w := g.FileWriter(filepath.Clean(g.tokenFile(file).Name()))
			if w == nil {
				continue
			}

			err = rewrite(pkg, file)
			if err != nil {
				return
			}

			err = g.writeNode(w, file)
			if err != nil {
				return
			}
		}
	}

	return nil
}

func (g Gen) tokenFile(node ast.Node) *token.File {
	return g.Loader.Fset.File(node.Pos())
}

func (g Gen) log(file *ast.File, node ast.Node, pattern string, args ...interface{}) {
	if g.Verbose == false {
		return
	}

	if file == nil && node == nil {
		fmt.Fprintf(os.Stderr, pattern+"\n", args...)
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

func (g Gen) showNode(node ast.Node) string {
	var buf bytes.Buffer
	format.Node(&buf, g.Loader.Fset, node)
	return buf.String()
}
