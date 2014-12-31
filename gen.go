package gen

import (
	"io"
	"path/filepath"

	"go/ast"
	"go/format"
	"go/parser"
	"golang.org/x/tools/astutil"
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

	ssaProg *ssa.Program
}

func (g *Gen) RewriteFiles(filenames []string) error {
	g.SourceImports = true
	g.ParserMode = parser.ParseComments

	var err error

	err = g.CreateFromFilenames("", filenames...)
	if err != nil {
		return err
	}

	err = g.initProg()
	if err != nil {
		return err
	}

	return g.rewriteProg()
}

func (g *Gen) initProg() error {
	var err error
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

func mkCallGraphInEdges(ssaPkg *ssa.Package, file *ast.File, funcDecl *ast.FuncDecl) []*callgraph.Edge {
	conf := &pointer.Config{
		BuildCallGraph: true,
		Mains:          []*ssa.Package{ssaPkg},
	}

	ptAnalysis, err := pointer.Analyze(conf)
	if err != nil {
		panic(err) // TODO
	}

	path, _ := astutil.PathEnclosingInterval(file, funcDecl.Pos(), funcDecl.End())
	ssaFn := ssa.EnclosingFunction(ssaPkg, path)

	return ptAnalysis.CallGraph.CreateNode(ssaFn).In
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

func argsTypesAt(pos int, edges []*callgraph.Edge) []types.Type {
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

func (g *Gen) rewriteFile(pkgInfo *loader.PackageInfo, file *ast.File) error {
	ssaPkg := g.ssaProg.Package(pkgInfo.Pkg)

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// for each type switch statements...
		for _, stmt := range funcDecl.Body.List {
			sw, ok := stmt.(*ast.TypeSwitchStmt)
			if !ok {
				continue
			}

			typeSwitch := NewTypeSwitchStmt(sw, pkgInfo.Info)
			if typeSwitch == nil {
				continue
			}

			target := typeSwitch.Target()

			// TODO check target is an interface{}

			// XXX parentScope must be of a func
			// scope := pkgInfo.Scopes[sw]
			// parentScope, _ := scope.LookupParent(target.Name)
			// assert(pkgInfo.Scopes[funcDecl.Type] == parentScope)

			// argument index of the variable which is target of the type switch
			pos := namedParamPos(target.Name, funcDecl.Type.Params)
			in := mkCallGraphInEdges(ssaPkg, file, funcDecl)
			inTypes := argsTypesAt(pos, in)

			// Finally rewrite it
			*sw = *typeSwitch.Expand(inTypes)
		}
	}

	return nil
}

func (g *Gen) rewriteProg() error {
	for _, pkgInfo := range g.Prog.Created {
		for _, file := range pkgInfo.Files {
			w := g.FileWriter(filepath.Clean(g.Fset.File(file.Pos()).Name()))
			if w == nil {
				continue
			}

			var err error
			err = g.rewriteFile(pkgInfo, file)
			if err != nil {
				return err
			}

			err = g.writeNode(w, file)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
