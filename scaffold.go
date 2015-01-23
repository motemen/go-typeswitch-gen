package gen

import (
	"fmt"
	"strings"

	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
)

func forTypeSwitchStmt(file *ast.File, proc func(*ast.FuncDecl, *ast.TypeSwitchStmt) error) error {
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

			err := proc(funcDecl, sw)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

var _stubStmt ast.Stmt

func stubStmt() ast.Stmt {
	if _stubStmt != nil {
		return _stubStmt
	}

	e, err := parser.ParseExpr(`panic("not implemented")`)
	if err != nil {
		panic(err)
	}

	_stubStmt = &ast.ExprStmt{e}
	return _stubStmt
}

func (g Gen) scaffoldFile(pkg *loader.PackageInfo, file *ast.File) error {
	return forTypeSwitchStmt(file, func(fd *ast.FuncDecl, sw *ast.TypeSwitchStmt) error {
		stmt := newTypeSwitchStmt(file, sw, pkg.Info)

		subjType := pkg.Info.TypeOf(stmt.subject())
		subjIf, ok := subjType.Underlying().(*types.Interface)
		if !ok {
			return fmt.Errorf("not an interface type: %v", subjType)
		}

		if subjIf.NumMethods() == 0 { // or use types.MethodSetCache?
			return fmt.Errorf("not implemented: type swithces on interface{}")
		}

		// List possible type cases
		candTypes := []types.Type{}
		for _, t := range g.allNamedTypes() {
			if _, isIf := t.Underlying().(*types.Interface); isIf {
				continue
			}

			if types.AssignableTo(t, subjIf) {
				candTypes = append(candTypes, t)
			}

			if pt := types.NewPointer(t); types.AssignableTo(pt, subjIf) {
				candTypes = append(candTypes, pt)
			}
		}

		cases := stmt.caseTypes()

		for _, t := range candTypes {
			var existing bool
			for et := range cases {
				existing = existing || types.Identical(t, et)
			}
			if !existing {
				typeString, path := splitType(t)

				// FIXME ad-hoc
				if path == file.Name.Name {
					s := strings.Index(typeString, "*") + 1
					e := strings.Index(typeString, ".")
					typeString = typeString[0:s] + typeString[e+1:]
				} else {
					addImport(file, path)
				}

				expr, err := parser.ParseExpr(typeString)
				if err != nil {
					panic(err)
				}

				newClause := &ast.CaseClause{
					List: []ast.Expr{expr},
					Body: []ast.Stmt{stubStmt()},
				}
				stmt.node.Body.List = append(stmt.node.Body.List, newClause)
			}
		}

		return nil
	})
}

// addImport modifies the ast.File file to add import path
func addImport(file *ast.File, path string) {
	if path == file.Name.Name {
		// The import path just specifies the file itself
		return
	}

	if file.Imports == nil {
		file.Imports = []*ast.ImportSpec{}
	}

	for _, importSpec := range file.Imports {
		if path == importSpec.Path.Value {
			return
		}
	}

	spec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: path,
		},
	}
	file.Imports = append(file.Imports, spec)
}

// allNamed returns all named types declared or loaded inside
// the program, plus built-in error type.
// (as oracle tool does)
func (g Gen) allNamedTypes() []types.Type {
	all := []types.Type{}

	for _, info := range g.program.AllPackages {
		for _, obj := range info.Defs {
			if tn, ok := obj.(*types.TypeName); ok {
				all = append(all, tn.Type())
			}
		}
	}

	all = append(all, types.Universe.Lookup("error").Type())

	return all
}
