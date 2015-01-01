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
	var err error

	out := new(bytes.Buffer)
	g := Gen{
		Verbose: testing.Verbose(),
		FileWriter: func(path string) io.WriteCloser {
			assert.Equal(t, "testdata/e.go", path)
			return nopCloser{out}
		},
	}
	err = g.CreateFromFilenames("", "./testdata/e.go")
	assert.NoError(t, err)

	err = g.RewriteFiles()
	assert.NoError(t, err)

	t.Log(out.String())
}
