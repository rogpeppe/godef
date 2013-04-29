package main

import (
	"bufio"
	"fmt"
	"go/build"
	. "launchpad.net/gocheck"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestPackage(t *testing.T) {
	TestingT(t)
}

type suite struct {
	savedBuildContext build.Context
	savedErrorf       func(string, ...interface{})
	errors            []string
}

var _ = Suite(&suite{})

func (s *suite) SetUpTest(c *C) {
	s.savedBuildContext = buildContext
	s.savedErrorf = errorf
	errorf = func(f string, a ...interface{}) {
		s.errors = append(s.errors, fmt.Sprintf(f, a...))
	}
}

func (s *suite) TearDownTest(c *C) {
	buildContext = s.savedBuildContext
	errorf = s.savedErrorf
	s.errors = nil
}

type listResult struct {
	project string
}

var listTests = []struct {
	about    string
	args     []string
	testDeps bool
	result   string
	errors   []string
}{{
	about: "easy case",
	args:  []string{"foo/foo1"},
	result: `
bar bzr 1
foo hg 0
foo/foo2 hg 0
`[1:],
}, {
	about:    "with test dependencies",
	args:     []string{"foo/foo1"},
	testDeps: true,
	result: `
bar bzr 1
baz bzr 1
foo hg 0
foo/foo2 hg 0
khroomph bzr 1
`[1:],
}}

func (s *suite) TestList(c *C) {
	dir := c.MkDir()
	gopath := []string{filepath.Join(dir, "p1"), filepath.Join(dir, "p2")}
	writePackages(c, gopath[0], "v1", map[string]packageSpec{
		"foo/foo1": {
			deps:      []string{"foo/foo2"},
			testDeps:  []string{"baz/baz1"},
			xTestDeps: []string{"khroomph/khr"},
		},
		"foo/foo2": {
			deps: []string{"bar/bar1"},
		},
		"baz/baz1":     {},
		"khroomph/khr": {},
	})
	writePackages(c, gopath[1], "v1", map[string]packageSpec{
		"bar/bar1": {
			deps: []string{"foo/foo3", "bar/bar2"},
		},
		"bar/bar2": {
			deps: []string{"bar/bar3"},
		},
		"bar/bar3": {},
		"bar/bar4": {},
		"foo/foo1": {},
		"foo/foo3": {},
	})
	var wg sync.WaitGroup
	goInitRepo := func(kind string, rootDir string, pkg string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			initRepo(c, kind, rootDir, pkg)
		}()
	}
	goInitRepo("bzr", gopath[0], "foo/foo1")
	goInitRepo("hg", gopath[0], "foo/foo2")
	goInitRepo("bzr", gopath[0], "baz")
	goInitRepo("bzr", gopath[0], "khroomph")
	goInitRepo("bzr", gopath[1], "bar")
	goInitRepo("hg", gopath[1], "foo")
	wg.Wait()

	buildContext.GOPATH = strings.Join(gopath, string(filepath.ListSeparator))

	for i, test := range listTests {
		c.Logf("test %d. %s", i, test.about)
		deps := list([]string{"foo/foo1"}, test.testDeps)
		c.Check(s.errors, HasLen, 0)

		// Check that rev ids are non-empty, but don't check specific values.
		result := ""
		for i, info := range deps {
			c.Check(info.revid, Not(Equals), "", Commentf("info %d: %v", i, info))
			info.revid = ""
			result += fmt.Sprintf("%s %s %s\n", info.project, info.vcs.Kind(), info.revno)
		}
		c.Check(result, Equals, test.result)
	}
}

func initRepo(c *C, kind, rootDir, pkg string) {
	// This relies on the fact that hg, bzr and git
	// all use the same command to initialize a directory.
	dir := filepath.Join(rootDir, "src", filepath.FromSlash(pkg))
	_, err := runCmd(dir, kind, "init")
	if !c.Check(err, IsNil) {
		return
	}
	_, err = runCmd(dir, kind, "add", dir)
	if !c.Check(err, IsNil) {
		return
	}
	commitRepo(c, dir, kind, "initial commit")
}

func commitRepo(c *C, dir, kind string, message string) {
	// This relies on the fact that hg, bzr and git
	// all use the same command to initialize a directory.
	_, err := runCmd(dir, kind, "commit", "-m", message)
	c.Check(err, IsNil)
}

type packageSpec struct {
	deps      []string
	testDeps  []string
	xTestDeps []string
}

func writePackages(c *C, rootDir string, version string, pkgs map[string]packageSpec) {
	srcDir := filepath.Join(rootDir, "src")
	for name, pkg := range pkgs {
		pkgDir := filepath.Join(srcDir, name)
		err := os.MkdirAll(pkgDir, 0777)
		c.Assert(err, IsNil)
		writeFile := func(fileName, pkgIdent string, deps []string) {
			err := writePackageFile(filepath.Join(pkgDir, fileName), pkgIdent, version, deps)
			c.Assert(err, IsNil)
		}
		writeFile("x.go", "x", pkg.deps)
		writeFile("internal_test.go", "x", pkg.testDeps)
		writeFile("x_test.go", "x_test", pkg.xTestDeps)
	}
}

func writePackageFile(fileName string, pkgIdent string, version string, deps []string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "package %s\nimport (\n", pkgIdent)
	for _, dep := range deps {
		fmt.Fprintf(w, "\t_ %q\n", dep)
	}
	fmt.Fprintf(w, ")\n")
	fmt.Fprintf(w, "const Version = %q\n", version)
	return w.Flush()
}
