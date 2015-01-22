package gen

import (
	"fmt"
	"go/ast"
	"golang.org/x/tools/go/loader"
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

func (g Gen) scaffoldFile(pkg *loader.PackageInfo, file *ast.File) error {
	return forTypeSwitchStmt(file, func(fd *ast.FuncDecl, sw *ast.TypeSwitchStmt) error {
		stmt := newTypeSwitchStmt(file, sw, pkg.Info)
		target := pkg.Info.Uses[stmt.target()] // The object where the type switch statement target is defined
		if target.Parent() == pkg.Scopes[fd.Type] {
			fmt.Println(fd, target)
		}

		return nil
	})
}
