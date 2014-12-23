package gen

import (
	"fmt"

	"go/ast"
	"golang.org/x/tools/go/types"
)

type T interface{}

type SliceT []interface{}

type MapT map[interface{}]interface{}

type NumberT float64

type TypeSwitchStmt struct {
	Ast       *ast.TypeSwitchStmt
	Templates []Template
}

func (stmt TypeSwitchStmt) FindMatchingTemplate(in types.Type) (*Template, TypeMatchResult) {
	for _, t := range stmt.Templates {
		if m, ok := t.Matches(in); ok {
			return &t, m
		}
	}

	return nil, nil
}

type Template struct {
	Pattern     ast.Expr
	PatternType types.Type
	CaseClause  *ast.CaseClause
}

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

func (t *Template) Matches(in types.Type) (TypeMatchResult, bool) {
	m := TypeMatchResult{}
	if typeMatches(t.PatternType, in, m) {
		return m, true
	}

	return nil, false
}

func (t *Template) Apply(m TypeMatchResult) *ast.CaseClause {
	cc := apply(t.CaseClause, m)
	return cc
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
		copiedList := make([]ast.Stmt, len(node.List))
		for i, stmt := range node.List {
			copiedList[i] = copyNode(stmt).(ast.Stmt)
		}
		copied.List = copiedList
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

func apply(node *ast.CaseClause, m TypeMatchResult) *ast.CaseClause {
	n := copyNode(node).(*ast.CaseClause)
	ast.Inspect(n, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			if r, ok := m[ident.Name]; ok {
				ident.Name = r.String()
			}
		}
		return true
	})

	return n
}

type TypeMatchResult map[string]types.Type

func parseTypeSwitchStmt(st *ast.TypeSwitchStmt, info types.Info) *TypeSwitchStmt {
	templates := []Template{}

	for _, clause := range st.Body.List {
		clause := clause.(*ast.CaseClause) // must not fail

		if len(clause.List) != 1 { // XXX should/can we support multiple patterns?
			continue
		}

		tmpl := Template{
			Pattern:     clause.List[0],
			PatternType: info.TypeOf(clause.List[0]),
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
