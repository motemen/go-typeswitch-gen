package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"go/format"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/loader"

	"github.com/motemen/go-typeswitch-gen"
)

func dieIf(err error, message ...string) {
	if err != nil {
		fmt.Println(message, err)
		os.Exit(1)
	}
}

// tsgen [-func <funcname>] <file>
func main() {
	// g := gen.Gen{}
	// args, err := g.FromArgs(os.Args[1:], false)
	// dieIf(err)

	// flag.StringVar(&g.FuncName, "func", "", "template func name")
	// err = flag.CommandLine.Parse(args)
	// dieIf(err)

	// err = g.Rewrite(funcName)
	// dieIf(err)

	conf := loader.Config{}
	conf.ParserMode = parser.ParseComments
	conf.SourceImports = true

	args, err := conf.FromArgs(os.Args[1:], false)
	dieIf(err, "conf.FromArgs")

	_ = flag.String("func", "", "template func name")

	err = flag.CommandLine.Parse(args)
	dieIf(err)

	prog, err := conf.Load()
	dieIf(err, "conf.Load")

	gen.RewriteProg(prog)

	fmt.Println(showNode(prog.Fset, prog.Created[0].Files[0]))
}

func showNode(fset *token.FileSet, node interface{}) string {
	var buf bytes.Buffer
	format.Node(&buf, fset, node)
	return buf.String()
}
