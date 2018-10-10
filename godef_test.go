package main

import (
	"go/build"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/godef/go/types"
	"golang.org/x/tools/go/packages/packagestest"
)

func TestGoDef(t *testing.T) { packagestest.TestAll(t, testGoDef) }
func testGoDef(t *testing.T, exporter packagestest.Exporter) {
	const godefAction = ">"
	modules := []packagestest.Module{{
		Name:  "github.com/rogpeppe/godef",
		Files: packagestest.MustCopyFileTree("testdata"),
	}}
	exported := packagestest.Export(t, exporter, modules)
	defer exported.Cleanup()

	posStr := func(p token.Position) string {
		return localPos(p, exported, modules)
	}

	const gopathPrefix = "GOPATH="
	const gorootPrefix = "GOROOT="
	for _, v := range exported.Config.Env {
		if strings.HasPrefix(v, gopathPrefix) {
			build.Default.GOPATH = v[len(gopathPrefix):]
		}
		if strings.HasPrefix(v, gorootPrefix) {
			build.Default.GOROOT = v[len(gorootPrefix):]
		}
	}

	count := 0
	exported.Expect(map[string]interface{}{
		"godef": func(src, target token.Position) {
			count++
			input, err := ioutil.ReadFile(src.Filename)
			if err != nil {
				t.Errorf("Failed %v: %v", src, err)
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
			obj, _, err := godef(src.Filename, input, src.Offset)
			if err != nil {
				t.Errorf("Failed %v: %v", src, err)
				return
			}
			pos := types.FileSet.Position(types.DeclPos(obj))
			check := token.Position{
				Filename: pos.Filename,
				Line:     pos.Line,
				Column:   pos.Column,
				Offset:   pos.Offset,
			}
			if posStr(check) != posStr(target) {
				t.Errorf("Got %v expected %v", posStr(check), posStr(target))
			}
		},
	})
	if count == 0 {
		t.Fatalf("No godef tests were run")
	}
}

var cwd, _ = os.Getwd()

func localPos(pos token.Position, e *packagestest.Exported, modules []packagestest.Module) string {
	fstat, fstatErr := os.Stat(pos.Filename)
	if fstatErr != nil {
		return pos.String()
	}
	for _, m := range modules {
		for fragment := range m.Files {
			fname := e.File(m.Name, fragment)
			if s, err := os.Stat(fname); err == nil && os.SameFile(s, fstat) {
				pos.Filename = filepath.Join(cwd, "testdata", filepath.FromSlash(fragment))
				return pos.String()
			}
		}
	}
	return pos.String()
}

