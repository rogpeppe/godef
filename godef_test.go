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

	"golang.org/x/tools/go/packages/packagestest"
)

func TestGoDef(t *testing.T) { packagestest.TestAll(t, testGoDef) }
func testGoDef(t *testing.T, exporter packagestest.Exporter) {
	files := packagestest.MustCopyFileTree("testdata")
	overlay := make(map[string][]byte)
	for fragment := range files {
		if trimmed := strings.TrimSuffix(fragment, ".overlay"); trimmed != fragment {
			delete(files, fragment)
			content, err := ioutil.ReadFile(filepath.Join("testdata", fragment))
			if err == nil {
				overlay[trimmed] = content
			}
		}
	}
	runGoDefTest(t, exporter, 1, []packagestest.Module{{
		Name:  "github.com/rogpeppe/godef",
		Files: files,
		Overlay: overlay,
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
			obj, err := invokeGodef(exported, src, runCount)
			if err != nil {
				t.Error(err)
				return
			}
			check := token.Position{
				Filename: obj.Position.Filename,
				Line:     obj.Position.Line,
				Column:   obj.Position.Column,
			}
			if check, target := localPos(check, exported), localPos(target, exported); check != target {
				t.Errorf("Got %v expected %v", check, target)
			}
		},
		"godefPrint": func(src token.Position, mode string, re *regexp.Regexp) {
			count++
			obj, err := invokeGodef(exported, src, runCount)
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

			print(buf, obj)
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

func invokeGodef(e *packagestest.Exported, src token.Position, runCount int) (*Object, error) {
	input, err := e.FileContents(src.Filename)
	if err != nil {
		return nil, fmt.Errorf("Failed %v: %v", src, err)
	}
	// repeat the actual godef part n times, for benchmark support
	var obj *Object
	for i := 0; i < runCount; i++ {
		obj, err = adaptGodef(e.Config, src.Filename, input, src.Offset)
		if err != nil {
			return nil, fmt.Errorf("Failed %v: %v", src, err)
		}
	}
	return obj, nil
}

func localPos(pos token.Position, e *packagestest.Exported) string {
	fstat, fstatErr := os.Stat(pos.Filename)
	if fstatErr != nil {
		return pos.String()
	}
	for _, m := range e.Modules {
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
