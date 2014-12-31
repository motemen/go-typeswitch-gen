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
		if len(message) > 0 {
			msg = strings.Join(message, " ") + ": " + msg
		}

		fmt.Println(msg)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`usage:
tsgen expand <file>
`)
	os.Exit(1)
}

// tsgen expand <file>
func main() {
	overwrite := flag.Bool("w", false, "write result to (source) file instead of stdout")
	flag.Parse()

	target := filepath.Clean(flag.Arg(0))
	if target == "" {
		usage()
	}

	var err error

	filenames, err := listSiblingFiles(target)
	dieIf(err)

	g := gen.Gen{}
	g.FileWriter = func(filename string) io.WriteCloser {
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

		return os.Stdout
	}

	err = g.RewriteFiles(filenames)
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
