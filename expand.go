package gen

import (
	"fmt"
	"strings"

	"go/ast"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"

	"github.com/motemen/go-astmanip"
)

// expandFileTypeSwitches is the main logic for "expand" mode.
// May rewrite type switch statements in *ast.File file.
func (g Gen) expandFileTypeSwitches(pkg *loader.PackageInfo, file *ast.File) error {
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

			typeSwitch := &typeSwitchStmt{
				file: file,
				node: sw,
				info: pkg.Info,
			}

			g.log(file, funcDecl, "enclosing func: %s", funcDecl.Type)

			inTypes, err := g.possibleSubjectTypes(pkg, funcDecl, typeSwitch)
			if err != nil {
				return err
			}

			for _, inType := range inTypes {
				// g.log(file, funcDecl, "argument type: %s (from %s)", inType, in[0].Caller.Func)
				g.log(file, funcDecl, "argument type: %s", inType)
			}

			// Finally rewrite it
			*sw = *g.expand(typeSwitch, inTypes)
		}
	}

	return nil
}

// typeSwitchStmt represents a parsed type switch statement.
type typeSwitchStmt struct {
	file *ast.File
	node *ast.TypeSwitchStmt
	info types.Info
}

// typeMatchResult is a type variable name to concrete type mapping
type typeMatchResult map[string]types.Type

func (stmt typeSwitchStmt) templates() []template {
	templates := []template{}

	for _, clause := range stmt.node.Body.List {
		clause := clause.(*ast.CaseClause) // must not fail

		if len(clause.List) != 1 { // XXX should/can we support multiple patterns?
			continue
		}

		tmpl := template{
			typePattern: stmt.info.TypeOf(clause.List[0]),
			caseClause:  clause,
		}
		templates = append(templates, tmpl)
	}

	return templates
}

// findMatchingTemplate finds the first matching template to the input type in and returns the template and a typeMatchResult.
func (gen Gen) findMatchingTemplate(stmt *typeSwitchStmt, in types.Type) (*template, typeMatchResult) {
	for _, t := range stmt.templates() {
		m := typeMatchResult{}
		if gen.typeMatches(stmt, t.typePattern, in, m) {
			return &t, m
		}
	}

	return nil, nil
}

// expand generates a type switch statement with expanded clauses for input types ins.
func (gen Gen) expand(stmt *typeSwitchStmt, ins []types.Type) *ast.TypeSwitchStmt {
	node := astmanip.CopyNode(stmt.node).(*ast.TypeSwitchStmt)
	seen := map[string]bool{}
	for _, in := range ins {
		if seen[in.String()] {
			continue
		}

		t, m := gen.findMatchingTemplate(stmt, in)
		if t == nil {
			// TODO error reporting
		}

		gen.log(stmt.file, stmt.node, "%s matched to %s -> %s", in, t.typePattern, m)

		clause := t.apply(m)
		node.Body.List = append(
			[]ast.Stmt{clause},
			node.Body.List...,
		)

		seen[in.String()] = true
	}

	return node
}

// subject returns the variable ast.Ident of interest of type-switch.
// TODO: support other forms than `switch y := x.(type)`, otherwise panics
func (stmt typeSwitchStmt) subject() *ast.Ident {
	return stmt.node.Assign.(*ast.AssignStmt).Rhs[0].(*ast.TypeAssertExpr).X.(*ast.Ident)
}

// caseTypes returns the map to clauses from their type cases.
func (stmt typeSwitchStmt) caseTypes() map[types.Type]*ast.CaseClause {
	cases := map[types.Type]*ast.CaseClause{}
	for _, cc := range stmt.node.Body.List {
		cc := cc.(*ast.CaseClause) // should not fail
		if cc.List == nil {        // the "default" case
			cases[nil] = cc
			continue
		}

		for _, e := range cc.List {
			t := stmt.info.TypeOf(e)
			cases[t] = cc
		}
	}

	return cases
}

// template represents a clause template.
type template struct {
	// typePattern is a type wich type variables e.g. map[string]T, func(T) (S, error).
	typePattern types.Type

	// caseClause is a clause template with type variables.
	caseClause *ast.CaseClause
}

// typeMatches is a helper function for FindMatchingTemplate
func (gen Gen) typeMatches(stmt *typeSwitchStmt, pat, in types.Type, m typeMatchResult) bool {
	switch pat := pat.(type) {
	case *types.Array:
		in, ok := in.(*types.Array)
		if !ok {
			return false
		}

		return gen.typeMatches(stmt, pat.Elem(), in.Elem(), m)

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

		return gen.typeMatches(stmt, pat.Elem(), in.Elem(), m)

	case *types.Interface:
		in, ok := in.(*types.Interface)
		if !ok {
			return false
		}

		// XXX is it OK?
		return types.Identical(pat, in)

	case *types.Map:
		in, ok := in.(*types.Map)
		if !ok {
			return false
		}

		if !gen.typeMatches(stmt, pat.Key(), in.Key(), m) {
			return false
		}
		if !gen.typeMatches(stmt, pat.Elem(), in.Elem(), m) {
			return false
		}

		return true

	case *types.Named:
		if gen.isTypeVariable(pat) {
			m[pat.Obj().Name()] = in
			return true
		}

		return pat.String() == in.String()

	case *types.Pointer:
		in, ok := in.(*types.Pointer)
		if !ok {
			return false
		}

		return gen.typeMatches(stmt, pat.Elem(), in.Elem(), m)

	case *types.Signature:
		in, ok := in.(*types.Signature)
		if !ok {
			return false
		}

		if !gen.typeMatches(stmt, pat.Params(), in.Params(), m) {
			return false
		}

		if !gen.typeMatches(stmt, pat.Results(), in.Results(), m) {
			return false
		}

		return true

	case *types.Slice:
		in, ok := in.(*types.Slice)
		if !ok {
			return false
		}

		return gen.typeMatches(stmt, pat.Elem(), in.Elem(), m)

	case *types.Struct:
		in, ok := in.(*types.Struct)
		if !ok {
			return false
		}

		if pat.NumFields() != in.NumFields() {
			return false
		}

		for i := 0; i < pat.NumFields(); i++ {
			if !gen.typeMatches(stmt, pat.Field(i).Type(), in.Field(i).Type(), m) {
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
			if !gen.typeMatches(stmt, pat.At(i).Type(), in.At(i).Type(), m) {
				return false
			}
		}

		return true

	default:
		fmt.Printf("TODO: %#v\n", pat)
		return false
	}
}

// apply applies typeMatchResult m to the template's caseClause and fills the type variables to specific types.
func (t *template) apply(m typeMatchResult) *ast.CaseClause {
	newClause := astmanip.CopyNode(t.caseClause).(*ast.CaseClause)
	ast.Inspect(newClause, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok {
			if r, ok := m[ident.Name]; ok {
				// TODO insert import; Here must be enhanced
				name, _ := splitType(r)
				ident.Name = name
			}
		}
		return true
	})

	return newClause
}

// splitType splits types.Type t to short form and its belonging package.
// e.g. type github.com/motemen/gen.Gen -> ("gen.Gen", "github.com/motemen/gen")
func splitType(t types.Type) (string, string) {
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		return obj.Pkg().Name() + "." + obj.Name(), obj.Pkg().Path()
	} else if pt, ok := t.(*types.Pointer); ok {
		name, pkg := splitType(pt.Elem())
		return "*" + name, pkg
	} else {
		return t.String(), ""
	}
}

// isTypeVariable checks if a named type is a type variable or not.
// Type variable is a type such that:
// - is an interface{} with name consisted of all uppercase letters
// - or a type declared with a comment of "// +tsgen typevar"
func (gen *Gen) isTypeVariable(t *types.Named) bool {
	if it, ok := t.Underlying().(*types.Interface); ok && it.Empty() {
		name := t.Obj().Name()
		if name == strings.ToUpper(name) {
			return true
		}
	}

	genDecls := []*ast.GenDecl{}

	for _, lpkg := range gen.program.Created {
		for _, file := range lpkg.Files {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if ok {
					genDecls = append(genDecls, genDecl)
				}
			}
		}
	}

	for _, genDecl := range genDecls {
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if isTypeVariableComment(genDecl.Doc) || isTypeVariableComment(typeSpec.Comment) {
				if typeSpec.Name.Name == t.Obj().Name() {
					return true
				}
			}
		}
	}

	return false
}

// isTypeVariableComment checks if cg is a comment like:
//   // +tsgen typevar
// or
//   /* +tsgen typevar */
// .
func isTypeVariableComment(cg *ast.CommentGroup) bool {
	if cg == nil {
		return false
	}

	for _, c := range cg.List {
		comment := strings.TrimSpace(c.Text[2:])
		if strings.HasPrefix(comment, "+tsgen typevar") {
			return true
		}
	}

	return false
}
