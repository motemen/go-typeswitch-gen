package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"go/ast"
	"go/build"
	"go/format"
	"go/token"

	"github.com/motemen/go-typeswitch-gen"
)

func dieIf(err error, message ...string) {
	if err != nil {
		msg := err.Error()
		if len(message) > 0 {
			msg = strings.Join(message, " ") + ": " + msg
		}

		fmt.Println(msg)
		os.Exit(1)
	}
}

// tsgen [-func <funcname>] <file>
func main() {
	g := gen.Gen{}

	flag.StringVar(&g.FuncName, "func", "", "template func name")
	flag.Parse()

	targetFilename := flag.Arg(0)

	var err error

	dir := filepath.Dir(targetFilename)
	entries, err := ioutil.ReadDir(dir)
	dieIf(err)

	filenames := []string{}
	for _, fi := range entries {
		match, err := build.Default.MatchFile(dir, fi.Name())
		dieIf(err)

		if match {
			filenames = append(filenames, filepath.Join(dir, fi.Name()))
		}
	}

	err = g.CreateFromFilenames("", filenames...)
	dieIf(err, "g.CreateFromFilenames")

	g.Target = func(fset *token.FileSet, file *ast.File) io.Writer {
		if filepath.Clean(fset.File(file.Pos()).Name()) == filepath.Clean(targetFilename) {
			return os.Stdout
		}

		return nil
	}

	err = g.RewriteFiles(filenames)
	dieIf(err)

	//for filename, astFile := range g.Files {
	//}

	//fmt.Println(showNode(g.Prog.Fset, g.Prog.Created[0].Files[0]))
}

func showNode(fset *token.FileSet, node interface{}) string {
	var buf bytes.Buffer
	format.Node(&buf, fset, node)
	return buf.String()
}
