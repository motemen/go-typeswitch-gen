package gen

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

type nopCloser struct {
	*bytes.Buffer
}

func (nc nopCloser) Close() error {
	return nil
}

func TestGen(t *testing.T) {
	out := new(bytes.Buffer)
	g := Gen{
		Verbose: testing.Verbose(),
		FileWriter: func(path string) io.WriteCloser {
			assert.Equal(t, "testdata/e.go", path)
			return nopCloser{out}
		},
	}
	err := g.RewriteFiles([]string{"./testdata/e.go"})
	assert.NoError(t, err)

	t.Log(out.String())
}
