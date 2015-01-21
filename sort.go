package gen

import (
	"path/filepath"
	"sort"

	"go/ast"
)

type byName struct {
	list []ast.Stmt
	gen  *Gen
}

func (s byName) Len() int      { return len(s.list) }
func (s byName) Swap(i, j int) { s.list[i], s.list[j] = s.list[j], s.list[i] }
func (s byName) Less(i, j int) bool {
	cc1 := s.list[i].(*ast.CaseClause)
	cc2 := s.list[j].(*ast.CaseClause)

	if cc1.List == nil {
		return false
	}
	if cc2.List == nil {
		return true
	}

	type1 := s.gen.showNode(cc1.List[0])
	type2 := s.gen.showNode(cc2.List[0])

	return type1 < type2
}

func (g *Gen) Sort() error {
	err := g.load()
	if err != nil {
		return err
	}

	for _, pkg := range g.program.AllPackages {
		for _, file := range pkg.Files {
			w := g.FileWriter(filepath.Clean(g.tokenFile(file).Name()))
			if w == nil {
				continue
			}

			ast.Inspect(file, func(n ast.Node) bool {
				if stmt, ok := n.(*ast.TypeSwitchStmt); ok {
					sort.Sort(byName{stmt.Body.List, g})
					return false
				}

				return true
			})

			err := g.writeNode(w, file)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
