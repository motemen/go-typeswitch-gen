package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"go/build"

	"github.com/motemen/go-typeswitch-gen"
)

func dieIf(err error, message ...string) {
	if err != nil {
		msg := err.Error()
		prefix := "fatal"
		if len(message) > 0 {
			prefix = strings.Join(message, " ")
		}

		fmt.Fprintln(os.Stderr, prefix+": "+msg)
		os.Exit(1)
	}
}

type noCloser struct {
	io.Writer
}

func (nc noCloser) Close() error {
	return nil
}

var usage = `Usage: %s [-w] [-main <pkg>] [-verbose] <mode> <file>

Modes:
  expand:   expand generic case clauses in type switch statements by its actual arguments
  sort:     sort case clauses in type switch statements
  scaffold: generate stub case clauses based on types that implement subject interface

Flags:
`

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}
}

func main() {
	var err error
	var (
		overwrite = flag.Bool("w", false, "write result to (source) file instead of stdout")
		verbose   = flag.Bool("verbose", false, "log verbose")
		main      = flag.String("main", "", "entrypoint package")
	)
	flag.Parse()

	args := flag.Args()

	if len(args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	mode := args[0]

	target := args[1]
	target, err = filepath.Abs(target)
	dieIf(err)

	if fi, err := os.Stat(target); err != nil || fi.IsDir() {
		flag.Usage()
		os.Exit(1)
	}

	g := gen.New()
	g.Verbose = *verbose
	g.FileWriter = func(filename string) io.WriteCloser {
		if filepath.IsAbs(filename) == false {
			// TODO check errors
			filename, _ = filepath.Abs(filename)
		}

		if filename != target {
			return nil
		}

		if *overwrite {
			w, err := os.Create(target)
			if err != nil {
				panic(err)
			}
			return w
		}

		return noCloser{os.Stdout}
	}

	switch mode {
	case "expand":
		err := doExpand(g, target, *main)
		dieIf(err)

	case "sort":
		err := doSort(g, target)
		dieIf(err)

	case "scaffold":
		err := doScaffold(g, target)
		dieIf(err)
	}
}

func doExpand(g *gen.Gen, target, main string) error {
	if main == "" {
		filenames, err := listSiblingFiles(target)
		if err != nil {
			return err
		}

		err = g.Loader.CreateFromFilenames("", filenames...)
		if err != nil {
			return err
		}
	} else {
		g.Loader.Import(main)
		g.Main = main
	}

	return g.Expand()
}

func doSort(g *gen.Gen, target string) error {
	if err := g.Loader.CreateFromFilenames("", target); err != nil {
		return err
	}

	return g.Sort()
}

func doScaffold(g *gen.Gen, target string) error {
	if err := g.Loader.CreateFromFilenames("", target); err != nil {
		return err
	}

	return g.Scaffold()
}

func listSiblingFiles(filename string) ([]string, error) {
	dir := filepath.Dir(filename)
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	filenames := []string{}
	for _, fi := range entries {
		match, err := build.Default.MatchFile(dir, fi.Name())
		if err != nil {
			return nil, err
		}

		if match {
			filenames = append(filenames, filepath.Join(dir, fi.Name()))
		}
	}

	return filenames, nil
}
