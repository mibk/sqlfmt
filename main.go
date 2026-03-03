package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"mibk.dev/sqlfmt/ast"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: sqlfmt [-w] [path ...]\n")
	fmt.Fprintf(os.Stderr, "  -w	write result to (source) file instead of stdout\n")
	os.Exit(2)
}

var (
	inPlace      = flag.Bool("w", false, "write to file")
	simplifyFlag = flag.Bool("s", false, "simplify code")
)

func main() {
	log.SetPrefix("sqlfmt: ")
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() == 0 {
		if *inPlace {
			log.Fatal("cannot use -w with standard input")
		}
		if err := formatCode("<stdin>", os.Stdout, os.Stdin); err != nil {
			log.Fatal(err)
		}
		return
	}

	for _, filename := range flag.Args() {
		f, err := os.Open(filename)
		if err != nil {
			log.Fatal(err)
		}
		fi, err := f.Stat()
		if err != nil {
			log.Fatal(err)
		}

		if !fi.IsDir() {
			if err := formatFile(filename, fi.Mode().Perm(), f); err != nil {
				log.Fatal(err)
			}
			continue
		}

		err = filepath.WalkDir(filename, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				log.Fatal(err)
			}
			if d.IsDir() {
				return nil
			}
			switch filepath.Ext(d.Name()) {
			default:
				return nil
			case ".sql":
			}

			f, err := os.Open(path)
			if err != nil {
				return err
			}
			return formatFile(path, d.Type().Perm(), f)
		})
		if err != nil {
			log.Fatal(err)
		}

	}
}

func formatFile(path string, perm fs.FileMode, data io.ReadCloser) error {
	buf := new(bytes.Buffer)
	if err := formatCode(path, buf, data); err != nil {
		return err
	}
	if err := data.Close(); err != nil {
		return err
	}

	if *inPlace {
		return os.WriteFile(path, buf.Bytes(), perm)
	}
	_, err := io.Copy(os.Stdout, buf)
	return err
}

func formatCode(filename string, out io.Writer, in io.Reader) error {
	script, err := ast.ParseScript(in)
	if se, ok := err.(*ast.SyntaxError); ok {
		return fmt.Errorf("%s:%d:%d: %v", filename, se.Line, se.Column, se.Err)
	} else if err != nil {
		return err
	}
	if *simplifyFlag {
		if err := ast.Simplify(script); err != nil {
			return err
		}
	}
	return ast.Fprint(out, script)
}
