package gen

import (
	"fmt"
	"strings"

	"go/ast"
	"golang.org/x/tools/go/types"
)

// TypeSwitchStmt represents a parsed type switch statement.
type TypeSwitchStmt struct {
	gen       *Gen
	file      *ast.File
	Ast       *ast.TypeSwitchStmt
	Templates []Template
}

type TypeMatchResult map[string]types.Type

func NewTypeSwitchStmt(gen *Gen, file *ast.File, st *ast.TypeSwitchStmt, info types.Info) *TypeSwitchStmt {
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
		gen:       gen,
		file:      file,
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

// Expand generates a type switch statement with expanded clauses for input types ins.
func (stmt TypeSwitchStmt) Expand(ins []types.Type) *ast.TypeSwitchStmt {
	node := copyNode(stmt.Ast).(*ast.TypeSwitchStmt)
	for _, in := range ins {
		t, m := stmt.FindMatchingTemplate(in)
		if t == nil {
			// TODO error reporting
		}

		stmt.gen.log(stmt.file, stmt.Ast, "%s matched to %s -> %s", in, t.TypePattern, m)

		clause := t.Apply(m)
		node.Body.List = append(
			[]ast.Stmt{clause},
			node.Body.List...,
		)
	}

	return node
}

// Target returns the variable ast.Ident of interest of type-switch.
// TODO: support other forms than `switch y := x.(type)`, otherwise panics
func (stmt TypeSwitchStmt) Target() *ast.Ident {
	return stmt.Ast.Assign.(*ast.AssignStmt).Rhs[0].(*ast.TypeAssertExpr).X.(*ast.Ident)
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
		if isTypeVariable(pat) {
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

// isTypeVariable checks if a named type is a type variable or not.
// Type variable is a type such that:
// - is an interface{} with name consisted of all uppercase letters
// - TODO: or a type with a comment of "// +tsgen: typevar"
func isTypeVariable(t *types.Named) bool {
	if it, ok := t.Underlying().(*types.Interface); ok && it.Empty() {
		name := t.Obj().Name()
		return name == strings.ToUpper(name)
	}

	return false
}
