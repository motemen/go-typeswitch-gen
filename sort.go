package gen

import (
	"go/ast"
	"golang.org/x/tools/go/types"
	"sort"
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

// Sort case clauses by popularity (most polular to less)
func (g *Gen) byInterface(list []ast.Stmt, info *types.Info) byInterfacePopularity {
	// First rank interfaces by their ocurrances
	caseTypes := map[types.Type]bool{}

	for _, st := range list {
		cc := st.(*ast.CaseClause)
		if cc.List == nil {
			continue
		}

		// We assume the case clause is inside a type switch statement
		// and the List has at most one element which is a type expression.
		caseTypes[info.TypeOf(cc.List[0])] = true
	}

	// Count all interfaces' implementation counts
	implCounts := map[*types.Interface]int{}
	for _, info := range g.program.AllPackages {
		for _, obj := range info.Defs {
			if tn, ok := obj.(*types.TypeName); ok {
				if i, ok := tn.Type().Underlying().(*types.Interface); ok {
					implCounts[i] = 0
				}
			}
		}
	}

	interfaceOrder := []*types.Interface{}
	for i := range implCounts {
		for t := range caseTypes {
			if types.Implements(t, i) {
				implCounts[i] = implCounts[i] + 1
			}
		}
		if implCounts[i] > 0 {
			interfaceOrder = append(interfaceOrder, i)
		}
	}

	sort.Sort(byImplCount{interfaceOrder, implCounts})

	return byInterfacePopularity{
		list:       list,
		interfaces: interfaceOrder,
		gen:        g,
	}
}

type byImplCount struct {
	interfaces []*types.Interface
	count      map[*types.Interface]int
}

func (s byImplCount) Len() int { return len(s.interfaces) }
func (s byImplCount) Swap(i, j int) {
	s.interfaces[i], s.interfaces[j] = s.interfaces[j], s.interfaces[i]
}
func (s byImplCount) Less(i, j int) bool {
	return s.count[s.interfaces[i]] > s.count[s.interfaces[j]]
}

type byInterfacePopularity struct {
	list       []ast.Stmt
	interfaces []*types.Interface
	gen        *Gen
	info       *types.Info
}

func (s byInterfacePopularity) Len() int { return len(s.list) }
func (s byInterfacePopularity) Swap(i, j int) {
	s.list[i], s.list[j] = s.list[j], s.list[i]
}
func (s byInterfacePopularity) Less(i, j int) bool {
	l1 := s.list[i].(*ast.CaseClause).List
	l2 := s.list[j].(*ast.CaseClause).List

	if l1 == nil {
		return false
	}
	if l2 == nil {
		return true
	}

	e1, e2 := l1[0], l2[0]
	t1, t2 := s.info.TypeOf(e1), s.info.TypeOf(e2)

	for _, in := range s.interfaces {
		impl1 := types.Implements(t1, in)
		impl2 := types.Implements(t2, in)
		if impl1 != impl2 {
			return impl1
		}
	}

	return s.gen.showNode(e1) < s.gen.showNode(e2)
}
