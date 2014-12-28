package gen

import (
	"fmt"

	"go/ast"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/pointer"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types"
)

func HandleProg(prog *loader.Program) {
	for _, pkgInfo := range prog.Created {
		mode := ssa.SanityCheckFunctions
		ssaProg := ssa.Create(prog, mode)
		ssaProg.BuildAll()

		ssaPkg := ssaProg.Package(pkgInfo.Pkg)

		info := pkgInfo.Info
		for ident, obj := range pkgInfo.Defs {
			fn, ok := obj.(*types.Func)
			if !ok {
				continue
			}

			_, path, _ := prog.PathEnclosingInterval(obj.Pos(), obj.Pos())

			var fd *ast.FuncDecl
			for _, node := range path {
				var ok bool
				if fd, ok = node.(*ast.FuncDecl); ok {
					break
				}
			}

			fmt.Printf("%#v fn=%#v fd=%#v\n", ident.Name, fn, fd)

			conf := &pointer.Config{}
			conf.BuildCallGraph = true
			conf.Mains = []*ssa.Package{ssaPkg}

			ptAnalysis, err := pointer.Analyze(conf)
			if err != nil {
				panic(err)
			}

			ssaFn := ssa.EnclosingFunction(ssaPkg, path)
			in := ptAnalysis.CallGraph.CreateNode(ssaFn).In

			for _, edge := range in {
				site := edge.Site
				if site == nil {
					continue
				}

				for _, a := range site.Common().Args {
					fmt.Println(a)
					if mi, ok := a.(*ssa.MakeInterface); ok {
						fmt.Println(mi.X.Type())
					}
				}
			}
		}

		for _, file := range pkgInfo.Files {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}

				for _, stmt := range fd.Body.List {
					sw, ok := stmt.(*ast.TypeSwitchStmt)
					if !ok {
						continue
					}

					scope := pkgInfo.Scopes[sw]
					// XXX only accept `switch x := y.(type)` form
					x := sw.Assign.(*ast.AssignStmt).Rhs[0].(*ast.TypeAssertExpr).X.(*ast.Ident)
					// XXX parentScope must be of a func
					parentScope, o := scope.LookupParent(x.Name)
					fmt.Printf("%#v ~~> %#v %v\n", x, o, parentScope)

					var outerFunc *ast.FuncType
					for node, s := range pkgInfo.Scopes {
						if s == parentScope {
							fmt.Printf("parentnode %#v\n", node)
							outerFunc = node.(*ast.FuncType)
							break
						}
					}

					var pos int
					var found bool
					for _, p := range outerFunc.Params.List {
						for _, n := range p.Names {
							if n.Name == x.Name {
								found = true
								break
							}
							pos = pos + 1
						}
					}
					fmt.Printf("%v, %v = %v th\n", found, x.Name, pos)

					// ここで y に対応する func arg の index を見つけて callgraph.In とつきあわせ、どんなインスタンスがあるか知りたい

					stmt := NewTypeSwitchStmt(sw, info)
					if stmt == nil {
						continue
					}
				}
			}
		}
	}
}

// TypeSwitchStmt represents a parsed type switch statement.
type TypeSwitchStmt struct {
	Ast       *ast.TypeSwitchStmt
	Templates []Template
}

type TypeMatchResult map[string]types.Type

func NewTypeSwitchStmt(st *ast.TypeSwitchStmt, info types.Info) *TypeSwitchStmt {
	templates := []Template{}

	for _, clause := range st.Body.List {
		clause := clause.(*ast.CaseClause) // must not fail

		if len(clause.List) != 1 { // XXX should/can we support multiple patterns?
			continue
		}

		tmpl := Template{
			TypePattern: info.TypeOf(clause.List[0]),
			CaseClause:  clause,
		}
		templates = append(templates, tmpl)
	}

	if len(templates) == 0 {
		return nil
	}

	return &TypeSwitchStmt{
		Ast:       st,
		Templates: templates,
	}
}

// FindMatchingTemplate find the first matching Template to the input type in and returns the Template and a TypeMatchResult.
func (stmt TypeSwitchStmt) FindMatchingTemplate(in types.Type) (*Template, TypeMatchResult) {
	for _, t := range stmt.Templates {
		if m, ok := t.Matches(in); ok {
			return &t, m
		}
	}

	return nil, nil
}

// Inflate generates a type switch statement with expanded clauses for input types ins.
func (stmt TypeSwitchStmt) Inflate(ins []types.Type) *ast.TypeSwitchStmt {
	node := copyNode(stmt.Ast).(*ast.TypeSwitchStmt)
	for _, in := range ins {
		t, m := stmt.FindMatchingTemplate(in)
		if t == nil {
			// TODO error reporting
		}
		clause := t.Apply(m)
		node.Body.List = append(
			[]ast.Stmt{clause},
			node.Body.List...,
		)
	}

	return node
}

// Template represents a clause template.
type Template struct {
	// TypePattern is a type wich type variables e.g. map[string]T, func(T) (S, error).
	TypePattern types.Type

	// CaseClause is a clause template with type variables.
	CaseClause *ast.CaseClause
}

// Matches tests whether input type in matches the template's TypePattern and returns a TypeMatchResult.
func (t *Template) Matches(in types.Type) (TypeMatchResult, bool) {
	m := TypeMatchResult{}
	if typeMatches(t.TypePattern, in, m) {
		return m, true
	}

	return nil, false
}

// typeMatches is a helper function for Matches.
func typeMatches(pat, in types.Type, m TypeMatchResult) bool {
	switch pat := pat.(type) {
	case *types.Array:
		panic("TODO *types.Array")

	case *types.Basic:
		return types.Identical(pat, in)

	case *types.Chan:
		in, ok := in.(*types.Chan)
		if !ok {
			return false
		}

		if pat.Dir() != in.Dir() {
			return false
		}

		return typeMatches(pat.Elem(), in.Elem(), m)

	case *types.Interface:
		panic("TODO *type.Interface")

	case *types.Map:
		in, ok := in.(*types.Map)
		if !ok {
			return false
		}

		if !typeMatches(pat.Key(), in.Key(), m) {
			return false
		}
		if !typeMatches(pat.Elem(), in.Elem(), m) {
			return false
		}

		return true

	case *types.Named:
		// TODO
		if pat.Obj().Name() == "T" || pat.Obj().Name() == "S" {
			// this is a type variable
			m[pat.Obj().Name()] = in
			return true
		}

		return pat.String() == in.String()

	case *types.Pointer:
		in, ok := in.(*types.Pointer)
		if !ok {
			return false
		}

		return typeMatches(pat.Elem(), in.Elem(), m)

	case *types.Signature:
		in, ok := in.(*types.Signature)
		if !ok {
			return false
		}

		if !typeMatches(pat.Params(), in.Params(), m) {
			return false
		}

		if !typeMatches(pat.Results(), in.Results(), m) {
			return false
		}

		return true

	case *types.Slice:
		in, ok := in.(*types.Slice)
		if !ok {
			return false
		}

		return typeMatches(pat.Elem(), in.Elem(), m)

	case *types.Struct:
		in, ok := in.(*types.Struct)
		if !ok {
			return false
		}

		if pat.NumFields() != in.NumFields() {
			return false
		}

		for i := 0; i < pat.NumFields(); i++ {
			if !typeMatches(pat.Field(i).Type(), in.Field(i).Type(), m) {
				return false
			}
		}

		return true

	case *types.Tuple:
		in, ok := in.(*types.Tuple)
		if !ok {
			return false
		}

		if pat.Len() != in.Len() {
			return false
		}

		for i := 0; i < pat.Len(); i++ {
			if !typeMatches(pat.At(i).Type(), in.At(i).Type(), m) {
				return false
			}
		}

		return true

	default:
		fmt.Printf("TODO: %#v\n", pat)
		return false
	}
}

// Apply applies TypeMatchResult m to the Template's CaseClause and fills the type variables to specific types.
func (t *Template) Apply(m TypeMatchResult) *ast.CaseClause {
	newClause := copyNode(t.CaseClause).(*ast.CaseClause)
	ast.Inspect(newClause, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok {
			if r, ok := m[ident.Name]; ok {
				ident.Name = r.String()
			}
		}
		return true
	})

	return newClause
}

func copyExprList(list []ast.Expr) []ast.Expr {
	if list == nil {
		return nil
	}

	copied := make([]ast.Expr, len(list))
	for i, expr := range list {
		copied[i] = copyNode(expr).(ast.Expr)
	}
	return copied
}

func copyStmtList(list []ast.Stmt) []ast.Stmt {
	if list == nil {
		return nil
	}

	copied := make([]ast.Stmt, len(list))
	for i, stmt := range list {
		copied[i] = copyNode(stmt).(ast.Stmt)
	}
	return copied
}

func copyFieldList(fl *ast.FieldList) *ast.FieldList {
	if fl == nil {
		return nil
	}

	copied := *fl

	if fl.List != nil {
		copiedList := make([]*ast.Field, len(fl.List))
		for i, f := range fl.List {
			field := *f
			field.Names = make([]*ast.Ident, len(f.Names))
			for i, name := range f.Names {
				copiedName := *name
				field.Names[i] = &copiedName
			}
			field.Type = copyNode(f.Type).(ast.Expr)
			copiedList[i] = &field
		}

		copied.List = copiedList
	}

	return &copied
}

// copyNode deep copies an ast.Node node.
func copyNode(node ast.Node) ast.Node {
	if node == nil {
		return nil
	}

	switch node := node.(type) {
	case *ast.DeclStmt:
		copied := *node // copy
		copied.Decl = copyNode(node.Decl).(ast.Decl)
		return &copied

	case *ast.GenDecl:
		copied := *node
		copiedSpecs := make([]ast.Spec, len(node.Specs))
		for i, spec := range node.Specs {
			copiedSpecs[i] = copyNode(spec).(ast.Spec)
		}
		copied.Specs = copiedSpecs
		return &copied

	case *ast.ValueSpec:
		copied := *node
		copied.Type = copyNode(node.Type).(ast.Expr)
		copied.Values = copyExprList(node.Values)
		return &copied

	case *ast.ArrayType:
		copied := *node
		copied.Elt = copyNode(node.Elt).(ast.Expr)
		return &copied

	case *ast.CallExpr:
		copied := *node
		copied.Args = copyExprList(node.Args)
		copied.Fun = copyNode(node.Fun).(ast.Expr)
		return &copied

	case *ast.Ident:
		copied := *node
		return &copied

	case *ast.IndexExpr:
		copied := *node
		return &copied

	case *ast.RangeStmt:
		copied := *node
		copied.Body = copyNode(node.Body).(*ast.BlockStmt)
		return &copied

	case *ast.AssignStmt:
		copied := *node
		copied.Lhs = copyExprList(node.Lhs)
		copied.Rhs = copyExprList(node.Rhs)
		return &copied

	case *ast.StarExpr:
		copied := *node
		copied.X = copyNode(node.X).(ast.Expr)
		return &copied

	case *ast.ExprStmt:
		copied := *node
		copied.X = copyNode(node.X).(ast.Expr)
		return &copied

	case *ast.SelectorExpr:
		copied := *node
		copied.X = copyNode(node.X).(ast.Expr)
		return &copied

	case *ast.BlockStmt:
		copied := *node
		copied.List = copyStmtList(node.List)
		return &copied

	case *ast.BasicLit:
		return node

	case *ast.SendStmt:
		copied := *node
		copied.Chan = copyNode(node.Chan).(ast.Expr)
		copied.Value = copyNode(node.Value).(ast.Expr)
		return &copied

	case *ast.CaseClause:
		copied := *node
		copied.List = copyExprList(node.List)
		copied.Body = copyStmtList(node.Body)
		return &copied

	case *ast.MapType:
		copied := *node
		copied.Key = copyNode(node.Key).(ast.Expr)
		copied.Value = copyNode(node.Value).(ast.Expr)
		return &copied

	case *ast.ChanType:
		copied := *node
		copied.Value = copyNode(node.Value).(ast.Expr)
		return &copied

	case *ast.FuncType:
		copied := *node
		copied.Params = copyFieldList(node.Params)
		copied.Results = copyFieldList(node.Results)
		return &copied

	case *ast.StructType:
		copied := *node
		copied.Fields = copyFieldList(node.Fields)
		return &copied

	default:
		fmt.Printf("copyNode: unexpected node type %T\n", node)
		return node
	}
}
