package main

import (
	"flag"
	"fmt"
	"go/parser"
	"os"

	"golang.org/x/tools/go/loader"

	"github.com/motemen/go-typeswitch-gen"
)

func dieIf(err error, message ...string) {
	if err != nil {
		fmt.Println(message, err)
		os.Exit(1)
	}
}

// tsgen -func <funcname> .
func main() {
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

	gen.HandleProg(prog)
}
