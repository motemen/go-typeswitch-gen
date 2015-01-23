package gen

import (
	"fmt"
	"go/ast"
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
		fmt.Printf("%#v", cases)

		for _, t := range candTypes {
			var existing bool
			for et := range cases {
				existing = existing || types.Identical(t, et)
			}
			if !existing {
				// TODO
				// stmt.addStubCaseClause(t)
				fmt.Println("TODO", t)
			}
		}

		return nil
	})
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
