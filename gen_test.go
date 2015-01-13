package gen

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"go/ast"

	"golang.org/x/tools/go/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nopCloser struct {
	*bytes.Buffer
}

func (nc nopCloser) Close() error {
	return nil
}

func TestGen(t *testing.T) {
	var err error

	out := new(bytes.Buffer)

	g := New()
	g.Verbose = testing.Verbose()
	g.FileWriter = func(path string) io.WriteCloser {
		if path == "testdata/e.go" {
			return nopCloser{out}
		}

		return nil
	}
	err = g.Loader.CreateFromFilenames("", "./testdata/e.go")
	assert.NoError(t, err)

	err = g.RewriteFiles()
	assert.NoError(t, err)

	t.Log(out.String())
}

func TestIsTypeVariable(t *testing.T) {
	gen := New()
	gen.Loader.CreateFromFilenames("", "testdata/types.go")

	err := gen.load()
	require.NoError(t, err)

	created := gen.program.Created[0]

	typeDefs := map[string]*types.Named{}
	for ident, obj := range created.Defs {
		if obj == nil {
			continue
		}

		typeDefs[ident.Name], _ = obj.Type().(*types.Named)
	}

	for _, file := range created.Files {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}

			doc := genDecl.Doc
			if doc == nil {
				continue
			}

			var isTypeVar bool
			for _, c := range doc.List {
				comment := strings.TrimSpace(c.Text[2:])
				if strings.HasPrefix(comment, "+tsgen typevar") {
					isTypeVar = true
					break
				}
			}

			if isTypeVar {
				for _, spec := range genDecl.Specs {
					t.Log(spec)
				}
			}
		}
	}

	assert.True(t, gen.isTypeVariable(typeDefs["T"]))
	assert.True(t, gen.isTypeVariable(typeDefs["NumberT"]))
	assert.False(t, gen.isTypeVariable(typeDefs["NonTypeVariableT"]))
}
