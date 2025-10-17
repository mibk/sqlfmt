package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rsc.io/diff"
)

var rewriteGolden = flag.Bool("f", false, "write golden files")

func TestFmt(t *testing.T) {
	files, err := filepath.Glob("testdata/*.input")
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range files {
		test := strings.TrimSuffix(name, ".input")
		t.Run(test, func(t *testing.T) {
			testFmt(t, name)
		})
	}
}

func testFmt(t *testing.T, filename string) {
	t.Helper()
	input, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	goldenName := strings.TrimSuffix(filename, ".input") + ".golden"
	golden, _ := ioutil.ReadFile(goldenName)

	t.Log(filename)
	t.Log(goldenName)

	got := fmtInput(t, input)

	if *rewriteGolden {
		os.WriteFile(goldenName, got, 0o644)
		return
	}

	if !bytes.Equal(got, golden) {
		diff := diff.Format(string(got), string(golden))
		t.Errorf("lines don't match (-got +want)\n%s", diff)
	}
}

func fmtInput(t *testing.T, src []byte) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	if err := formatCode("<test>", buf, bytes.NewReader(src)); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
	return buf.Bytes()
}
