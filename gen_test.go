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
	g := New()
	g.Verbose = testing.Verbose()
	g.FileWriter = func(path string) io.WriteCloser {
		if path == "testdata/e.go" {
			return nopCloser{out}
		} else {
			return nil
		}
	}
	err = g.Loader.CreateFromFilenames("", "./testdata/e.go")
	assert.NoError(t, err)

	err = g.RewriteFiles()
	assert.NoError(t, err)

	t.Log(out.String())
}
