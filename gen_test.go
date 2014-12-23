package gen

import (
	"bytes"
	"testing"

	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type typeMatchTestCase struct {
	patternType string
	matches     map[string]string
}

func TestTypeParams(t *testing.T) {
	code := `
package E

import (
	"io"
)

type T interface{}
type S interface{}

type xxx struct{}

type in1 map[string][]io.Reader
type in2 map[int]bool
type in3 []chan<- *xxx
type in4 []struct{}
type in5 *xxx
type in6 func(int)
type in7 func(bool) (io.Reader, error)
type in8 struct { foo []byte }

func Foo(x interface{}) {
	switch x := x.(type) {
	// in1
	case map[string]T:
		var r T // <-- T here
		for _, v := range x {
			r = v
		}
		_ = r

	// in2
	case map[T]bool:
		var keys []T = make([]T, 0)
		for k := range x {
			keys = append(keys, k)
		}
		_ = keys

	// in3
	case []chan<- T:
		var t1, t2 T
		for _, c := range x {
			c <- t1
			c <- t2
		}

	// in4
	case []T:
		var t T = x[0]
		_ = t

	// in5
	case *T:
		var t T = *x
		_ = t

	// in6
	case func(T):
		var t *T
		x(*t)

	// in7
	case func(T) (S, error):
		var t T
		var s S
		s, _ = x(t)
		_ = s

	// in8
	case struct { foo T }:
		var t T = x.foo
		_ = t
	}
}`

	conf := loader.Config{}
	conf.ParserMode = parser.ParseComments

	file, err := conf.ParseFile("test.go", code)
	require.NoError(t, err)

	conf.CreateFromFiles("", file)

	prog, err := conf.Load()
	require.NoError(t, err)

	typeDefs := map[string]types.Type{}

	for _, pkg := range prog.Created {
		for ident, obj := range pkg.Defs {
			if ty, ok := obj.(*types.TypeName); ok {
				typeDefs[ident.Name] = ty.Type().Underlying()
			}
		}
		require.Equal(t, "map[string][]io.Reader", typeDefs["in1"].String())

		cases := map[string]typeMatchTestCase{
			"in1": {
				"map[string]E.T",
				map[string]string{"T": "[]io.Reader"},
			},
			"in2": {
				"map[E.T]bool",
				map[string]string{"T": "int"},
			},
			"in3": {
				"[]chan<- E.T",
				map[string]string{"T": "*E.xxx"},
			},
			"in4": {
				"[]E.T",
				map[string]string{"T": "struct{}"},
			},
			"in5": {
				"*E.T",
				map[string]string{"T": "E.xxx"},
			},
			"in6": {
				"func(E.T)",
				map[string]string{"T": "int"},
			},
			"in7": {
				"func(E.T) (E.S, error)",
				map[string]string{"T": "bool", "S": "io.Reader"},
			},
			"in8": {
				"struct{foo E.T}",
				map[string]string{"T": "[]byte"},
			},
		}

		for node := range pkg.Scopes {
			sw, ok := node.(*ast.TypeSwitchStmt)
			if !ok {
				continue
			}

			stmt := parseTypeSwitchStmt(sw, pkg.Info)
			if stmt == nil {
				continue
			}

			for inTypeName, c := range cases {
				tmpl, m := stmt.FindMatchingTemplate(typeDefs[inTypeName])
				require.NotNil(t, tmpl, inTypeName)
				require.NotNil(t, m, inTypeName)
				assert.Equal(t, c.patternType, tmpl.PatternType.String(), inTypeName)

				for typeVar, ty := range c.matches {
					assert.Equal(t, ty, m[typeVar].String(), inTypeName)
				}

				newBody := tmpl.Apply(m)
				t.Log(showNode(prog.Fset, newBody))
			}

			t.Log(showNode(prog.Fset, sw))
		}
	}
}

func showNode(fset *token.FileSet, node interface{}) string {
	var buf bytes.Buffer
	printer.Fprint(&buf, fset, node)
	return buf.String()
}
