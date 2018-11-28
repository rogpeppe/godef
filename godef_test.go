package main

import (
	"bytes"
	"fmt"
	"go/build"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rogpeppe/godef/go/ast"
	"github.com/rogpeppe/godef/go/types"
	"golang.org/x/tools/go/packages/packagestest"
)

func TestGoDef(t *testing.T) { packagestest.TestAll(t, testGoDef) }
func testGoDef(t *testing.T, exporter packagestest.Exporter) {
	runGoDefTest(t, exporter, 1, []packagestest.Module{{
		Name:  "github.com/rogpeppe/godef",
		Files: packagestest.MustCopyFileTree("testdata"),
	}})
}

func BenchmarkGoDef(b *testing.B) { packagestest.BenchmarkAll(b, benchGoDef) }
func benchGoDef(b *testing.B, exporter packagestest.Exporter) {
	runGoDefTest(b, exporter, b.N, []packagestest.Module{{
		Name:  "github.com/rogpeppe/godef",
		Files: packagestest.MustCopyFileTree("testdata"),
	}})
}

func runGoDefTest(t testing.TB, exporter packagestest.Exporter, runCount int, modules []packagestest.Module) {
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
	if err := exported.Expect(map[string]interface{}{
		"godef": func(src, target token.Position) {
			count++
			obj, _, err := invokeGodef(src, runCount)
			if err != nil {
				t.Error(err)
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
		"godefPrint": func(src token.Position, mode string, re *regexp.Regexp) {
			count++
			obj, typ, err := invokeGodef(src, runCount)
			if err != nil {
				t.Error(err)
				return
			}
			buf := &bytes.Buffer{}
			switch mode {
			case "json":
				*jsonFlag = true
				*tflag = false
				*aflag = false
				*Aflag = false
			case "all":
				*jsonFlag = false
				*tflag = true
				*aflag = true
				*Aflag = true
			case "public":
				*jsonFlag = false
				*tflag = true
				*aflag = true
				*Aflag = false
			case "type":
				*jsonFlag = false
				*tflag = true
				*aflag = false
				*Aflag = false
			default:
				t.Fatalf("Invalid print mode %v", mode)
			}

			print(buf, obj, typ)
			if !re.Match(buf.Bytes()) {
				t.Errorf("in mode %q got %v want %v", mode, buf, re)
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

func invokeGodef(src token.Position, runCount int) (*ast.Object, types.Type, error) {
	input, err := ioutil.ReadFile(src.Filename)
	if err != nil {
		return nil, types.Type{}, fmt.Errorf("Failed %v: %v", src, err)
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
			return nil, types.Type{}, fmt.Errorf("cannot read saved file: %v", err)
		}
		if err := ioutil.WriteFile(src.Filename, savedData, 0666); err != nil {
			return nil, types.Type{}, fmt.Errorf("cannot write saved file: %v", err)
		}
		defer ioutil.WriteFile(src.Filename, input, 0666)
	}
	// repeat the actual godef part n times, for benchmark support
	var obj *ast.Object
	var typ types.Type
	for i := 0; i < runCount; i++ {
		obj, typ, err = godef(src.Filename, input, src.Offset)
		if err != nil {
			return nil, types.Type{}, fmt.Errorf("Failed %v: %v", src, err)
		}
	}
	return obj, typ, nil
}

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
