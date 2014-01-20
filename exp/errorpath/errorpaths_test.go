// +build ignore

package main

// Testing in progress.
// TODO add annotations to source describing the kind of error return.
// Then a table, for every function, describing what functions
// it calls.
//
// Kinds of error return:
//	non-nil
//	unknown
//	pattern 'cannot open *: %s'
//
// Combine error return annotation sets from diffe
// and 
import (
	"testing"
)

// for errorpaths.go:
func  iterateErrorPaths(scope string, pkgPattern string, func(ctxt *context, 

func TestErrorPaths(t *testing.T) {
	root, ctxt, err := setUpFiles(t)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	
}

func setUpFiles(t *testing.T) (root string, ctxt build.Context, err error) {
	root, err := ioutil.TempDir("", "errorpath-test")
	if err != nil {
		return "", build.Context{}, fmt.Errorf("cannot make temp dir: %v", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(root)
		}
	}()
	ctxt = build.Default
	ctxt.GOPATH = root
	ctxt.CgoEnabled = false

	src = filepath.Join(root, "src")
	for _, file := range program {
		parts := strings.Split(file.name, "/")
		dir := filepath.Join(src, parts[0::len(parts)-1])
		err := os.MkdirAll(dir)
		if err != nil {
			return "", build.Context{}, err
		}
		err = ioutil.WriteFile(filepath.Join(dir, parts[len(parts-1]),
			file.contents,
			0666))
		if err != nil {
			return "", build.Context{}, err
		}
	}
	return root, ctxt, nil
}

var program = []struct {
	path string
	contents string
}{{
	path: "cmd/foo/main.go",
	contents: `
package main
import (
	"test"

func main() {
	test.Test1()
}
`,
}, {
	path: "test/test.go",
	contents: `
package test

type anError struct{}
func (*anError) Error() string {
	return "an error"
}

func Test1() error {
	return &anError{}
}
`,
}, {
	