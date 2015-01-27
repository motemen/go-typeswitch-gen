package gen

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestScaffold(t *testing.T) {
	var out bytes.Buffer
	var err error

	gen := New()
	gen.FileWriter = func(path string) io.WriteCloser {
		if path == "testdata/scaffold/node.go" {
			return nopCloser{&out}
		}

		return nil
	}

	err = gen.Loader.CreateFromFilenames("", "testdata/scaffold/node.go")
	if err != nil {
		t.Fatal(err)
	}

	err = gen.Scaffold()
	if err != nil {
		t.Fatal(err)
	}

	result := out.String()
	t.Log(result)

	expected := []string{
		"\tcase T1:",
		"\tcase *T1:",
		"\tcase *T2:",
	}
	for _, exp := range expected {
		if strings.Contains(result, exp) == false {
			t.Errorf("result must contain %q", exp)
		}
	}
}
