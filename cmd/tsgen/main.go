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

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [<options>] <file> [-main <pkg>]\n", os.Args[0])
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

	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	target := flag.Arg(0)
	target, err = filepath.Abs(target)
	dieIf(err)

	if fi, err := os.Stat(target); err != nil || fi.IsDir() {
		flag.Usage()
		os.Exit(1)
	}

	g := gen.New()

	if *main == "" {
		filenames, err := listSiblingFiles(target)
		dieIf(err)

		err = g.Loader.CreateFromFilenames("", filenames...)
		dieIf(err)
	} else {
		g.Loader.Import(*main)
		g.Main = *main
	}

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

	err = g.RewriteFiles()
	dieIf(err)
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
