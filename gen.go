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
	Body        []ast.Stmt
	typeParams  map[string][]*ast.Ident
}

func (t *Template) init() {
	if t.typeParams != nil {
		return
	}

	visit := func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			if t.typeParams == nil {
				t.typeParams = map[string][]*ast.Ident{}
			}

			t.typeParams[n.Name] = append(t.typeParams[n.Name], n)
		}

		return true

	}

	for _, stmt := range t.Body {
		ast.Inspect(stmt, visit)
	}
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
		panic("TODO *types.Struct")

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

// not goroutine-safe
func (t *Template) Rewrite(m TypeMatchResult) {
	t.init()

	for n, ii := range t.typeParams {
		if r, ok := m[n]; ok {
			for _, i := range ii {
				i.Name = r.String()
			}
		}
	}
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
			Body:        clause.Body,
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
