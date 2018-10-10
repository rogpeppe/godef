package main

import (
	"go/build"
	"go/token"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/rogpeppe/godef/go/types"
	"golang.org/x/tools/go/packages/packagestest"
)

func TestGoDef(t *testing.T) {
	modules := []packagestest.Module{{
		Name:  "github.com/rogpeppe/godef",
		Files: packagestest.MustCopyFileTree("testdata"),
	}}
	exported := packagestest.Export(t, packagestest.GOPATH, modules)
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
			input, err := ioutil.ReadFile(src.Filename)
			if err != nil {
				t.Errorf("Failed %v: %v", src, err)
				return
			}
			obj, _, err := godef(src.Filename, input, src.Offset)
			if err != nil {
				t.Errorf("Failed %v: %v", src, err)
				return
			}
			pos := types.FileSet.Position(types.DeclPos(obj))
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
