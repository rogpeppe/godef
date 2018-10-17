package main

import (
	"go/token"
	"io/ioutil"
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
	if err := exported.Expect(map[string]interface{}{
		"godef": func(src, target token.Position) {
			count++
			input, err := ioutil.ReadFile(src.Filename)
			if err != nil {
				t.Errorf("Failed %v: %v", src, err)
				return
			}
			fSet, obj, err := godef(exported.Config, src.Filename, input, src.Offset)
			if err != nil {
				t.Errorf("Failed %v: %v", src, err)
				return
			}
			pos := objToPos(fSet, obj)
			if pos.String() != target.String() {
				t.Errorf("Got %v expected %v", pos, target)
			}
		},
	}); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatalf("No godef tests were run")
	}
}
