package main

import (
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages/packagestest"
)

func TestGoDef(t *testing.T) { packagestest.TestAll(t, testGoDef) }
func testGoDef(t *testing.T, exporter packagestest.Exporter) {
	modules := []packagestest.Module{{
		Name:  "github.com/rogpeppe/godef",
		Files: packagestest.MustCopyFileTree("testdata"),
	}}
	exported := packagestest.Export(t, packagestest.GOPATH, modules)
	defer exported.Cleanup()
	count := 0
	posStr := func(p token.Position) string {
		return localPos(p, exported, modules)
	}
	if err := exported.Expect(map[string]interface{}{
		"godef": func(src, target token.Position) {
			count++
			input, err := ioutil.ReadFile(src.Filename)
			if err != nil {
				t.Fatalf("cannot read source: %v", err)
				return
			}
			// There's a "saved" version of the file, so
			// copy it to the original version; we want the
			// Expect method to see the in-editor-buffer
			// versions of the files, but we want the godef
			// function to see the files as they should
			// be on disk, so that we're actually testing the
			// define-in-buffer functionality.
			savedFile := src.Filename + ".saved"
			if _, err := os.Stat(savedFile); err == nil {
				savedData, err := ioutil.ReadFile(savedFile)
				if err != nil {
					t.Fatalf("cannot read saved file: %v", err)
				}
				if err := ioutil.WriteFile(src.Filename, savedData, 0666); err != nil {
					t.Fatalf("cannot write saved file: %v", err)
				}
				defer ioutil.WriteFile(src.Filename, input, 0666)
			}
			fSet, obj, err := godef(exported.Config, src.Filename, input, src.Offset)
			if err != nil {
				t.Errorf("godef error %v: %v", posStr(src), err)
				return
			}
			pos := objToPos(fSet, obj)
			if pos.String() != target.String() {
				t.Errorf("unexpected result %v -> %v want %v", posStr(src), posStr(pos), posStr(target))
			}
		},
	}); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatalf("No godef tests were run")
	}
}

var cwd, _ = os.Getwd()

func localPos(pos token.Position, e *packagestest.Exported, modules []packagestest.Module) string {
	f := pos.Filename
	for _, m := range modules {
		md := filepath.FromSlash(m.Name)
		i := strings.LastIndex(f, md)
		if i == -1 {
			continue
		}
		f = f[i+len(md)+1:]
		pos.Filename = filepath.Join(cwd, "testdata", f)
		break
	}
	return pos.String()
}
