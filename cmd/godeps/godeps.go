package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var revFile = flag.String("u", "", "file containing desired revisions")
var testDeps = flag.Bool("t", false, "include testing dependencies in output")

var exitCode = 0

var buildContext = build.Default

var usage = `
Usage:
	godeps [-t] [pkg ...]
	godeps -u file

In the first form of usage, godeps prints to standard output a list of
all the source dependencies of the named packages (or the package in
the current directory if none is given).  If there is ambiguity in the
source-control systems used, godeps will print all the available versions
and an error, exiting with a false status. It is up to the user to remove
lines from the output to make the output suitable for input to godeps -u.

The -t flag specifies that testing dependencies will also be included.

In the second form, godeps updates source to versions specified by the
-u file argument, which should hold version information in the same
form printed by godeps. It is an error if the file contains more than
one line for the same package root.

`[1:]

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s", usage)
		os.Exit(2)
	}
	flag.Parse()
	if *revFile != "" {
		if flag.NArg() != 0 {
			flag.Usage()
		}
		update(*revFile)
	} else {
		pkgs := flag.Args()
		if len(pkgs) == 0 {
			pkgs = []string{"."}
		}
		for _, info := range list(pkgs, *testDeps) {
			fmt.Println(info)
		}
	}
	os.Exit(exitCode)
}

func update(file string) {
	errorf("update not yet implemented")
}

func list(pkgs []string, testDeps bool) []*depInfo {
	infoByDir := make(map[string][]*depInfo)
	// We want to ignore the go core source and the projects
	// for the root packages. Do this by getting leaf dependency info
	// for all those things and adding them to an ignore list.
	for _, pkgPath := range pkgs {
		pkg, err := buildContext.Import(pkgPath, ".", build.FindOnly)
		if err != nil {
			errorf("cannot find %q: %v", pkgPath, err)
			continue
		}
		findDepInfo(pkg.Dir, infoByDir)
	}
	ignoreDirs := map[string]bool{
		filepath.Clean(buildContext.GOROOT): true,
	}
	for dir := range infoByDir {
		ignoreDirs[dir] = true
	}
	walkDeps(pkgs, testDeps, func(pkg *build.Package, err error) bool {
		if err != nil {
			errorf("cannot import %q: %v", pkg.Name, err)
			return false
		}
		findDepInfo(pkg.Dir, infoByDir)
		return true
	})
	// We make a new map because dependency information
	// can be ambiguous not only through there being two
	// or more metadata directories in one directory, but
	// also because there can be different packages with
	// the same project name under different GOPATH
	// elements.
	infoByProject := make(map[string][]*depInfo)
	for dir, infos := range infoByDir {
		proj, err := dirToProject(dir)
		if err != nil && !ignoreDirs[dir] {
			errorf("cannot get relative repo root for %q: %v", dir, err)
			continue
		}
		infoByProject[proj] = append(infoByProject[proj], infos...)
	}
	var deps depInfoSlice
	for proj, infos := range infoByProject {
		if len(infos) > 1 {
			for _, info := range infos {
				errorf("ambiguous VCS for %q at %q", proj, info.dir)
			}
		}
		for _, info := range infos {
			if ignoreDirs[info.dir] {
				continue
			}
			info.project = proj
			deps = append(deps, info)
		}
	}
	sort.Sort(deps)
	return deps
}

func dirToProject(dir string) (string, error) {
	if ok, _ := relativeToParent(buildContext.GOROOT, dir); ok {
		return "go", nil
	}
	for _, p := range filepath.SplitList(buildContext.GOPATH) {
		if ok, rel := relativeToParent(filepath.Join(p, "src"), dir); ok {
			return rel, nil
		}
	}
	return "", fmt.Errorf("project directory not found in GOPATH or GOROOT")
}

// relativeToParent returns whether the child
// path is under (or the same as) the parent path,
// and if so returns the trailing portion of the
// child path that is under the parent path.
func relativeToParent(parent, child string) (ok bool, rel string) {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)

	if parent == child {
		return true, ""
	}

	if !strings.HasPrefix(child, parent+"/") {
		return false, ""
	}
	return true, child[len(parent)+1:]
}

type depInfoSlice []*depInfo

func (s depInfoSlice) Len() int      { return len(s) }
func (s depInfoSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s depInfoSlice) Less(i, j int) bool {
	p, q := s[i], s[j]
	if p.project != q.project {
		return p.project < q.project
	}
	return p.vcs.Kind() < q.vcs.Kind()
}

type depInfo struct {
	project string
	dir     string
	vcs     VCS
	VCSInfo
}

func (info *depInfo) String() string {
	return fmt.Sprintf("%s\t%s\t%s\t%s", info.project, info.vcs.Kind(), info.revid, info.revno)
}

func findDepInfo(dir string, infoByDir map[string][]*depInfo) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		errorf("cannot find absolute path of %q", dir)
		return
	}
	dirs := parents(dir)
	// Check from the root down that there is no
	// existing information for any parent directory.
	for i := len(dirs) - 1; i >= 0; i-- {
		if info := infoByDir[dirs[i]]; info != nil {
			return
		}
	}
	// Check from dir upwards to find an SCS directory
	for _, dir := range dirs {
		nfound := 0
		for metaDir, vcs := range metadataDirs {
			if dirInfo, err := os.Stat(filepath.Join(dir, metaDir)); err == nil && dirInfo.IsDir() {
				info, err := vcs.Info(dir)
				if err != nil {
					errorf("cannot get version information for %q: %v", dir, err)
					continue
				}
				infoByDir[dir] = append(infoByDir[dir], &depInfo{
					dir:     dir,
					vcs:     vcs,
					VCSInfo: info,
				})
				nfound++
			}
		}
		if nfound > 0 {
			return
		}
	}
	errorf("no version control system found for %q", dir)
}

// parents returns the given path and all its parents.
// For instance, given /usr/rog/foo,
// it will return []string{"/usr/rog/foo", "/usr/rog", "/usr", "/"}
func parents(path string) []string {
	var all []string
	path = filepath.Clean(path)
	for {
		all = append(all, path)
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return all
}

type walkContext struct {
	checked      map[string]bool
	includeTests bool
	visit        func(*build.Package, error) bool
}

// walkDeps traverses the import dependency tree of the
// given package, calling the given function for each dependency,
// including the package for pkgPath itself. If the function
// returns true, the dependencies of the given package
// will themselves be visited.
// The includeTests flag specifies whether test-related dependencies
// will be considered when walking the hierarchy.
// Each package will be visited at most once.
func walkDeps(paths []string, includeTests bool, visit func(*build.Package, error) bool) {
	ctxt := &walkContext{
		checked:      make(map[string]bool),
		includeTests: includeTests,
		visit:        visit,
	}
	for _, path := range paths {
		ctxt.walkDeps(path)
	}
}

func (ctxt *walkContext) walkDeps(pkgPath string) {
	if pkgPath == "C" {
		return
	}
	if ctxt.checked[pkgPath] {
		// The package has already been, is or being, checked
		return
	}
	// BUG(rog) This ignores files that are excluded by
	// as part of the current build. Unfortunately
	// we can't use UseAllFiles as that includes other
	// files that break the build (for instance unrelated
	// helper commands in package main).
	// The solution is to avoid using build.Import but it's convenient
	// at the moment.
	pkg, err := buildContext.Import(pkgPath, ".", 0)
	ctxt.checked[pkg.ImportPath] = true
	descend := ctxt.visit(pkg, err)
	if err != nil || !descend {
		return
	}
	// N.B. is it worth eliminating duplicates here?
	var allImports []string
	allImports = append(allImports, pkg.Imports...)
	if ctxt.includeTests {
		allImports = append(allImports, pkg.TestImports...)
		allImports = append(allImports, pkg.XTestImports...)
	}
	for _, impPath := range allImports {
		ctxt.walkDeps(impPath)
	}
}

type VCS interface {
	Kind() string
	Info(dir string) (VCSInfo, error)
	Update(dir, info string) error
}

type VCSInfo struct {
	revid string
	revno string // optional
}

var metadataDirs = map[string]VCS{
	".bzr": bzrVCS{},
	".hg":  hgVCS{},
}

// TODO git
// git rev-parse HEAD
// git checkout $revid

type bzrVCS struct{}

func (bzrVCS) Kind() string {
	return "bzr"
}

var validBzrInfo = regexp.MustCompile(`^([0-9]+) ([^ \t]+)$`)

func (bzrVCS) Info(dir string) (VCSInfo, error) {
	out, err := runCmd(dir, "bzr", "revision-info", "--tree")
	if err != nil {
		return VCSInfo{}, err
	}
	m := validBzrInfo.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return VCSInfo{}, fmt.Errorf("bzr revision-info has unexpected result %q", out)
	}
	// TODO(rog) check that tree is clean
	return VCSInfo{
		revid: m[2],
		revno: m[1],
	}, nil
}

func (bzrVCS) Update(dir string, revid string) error {
	_, err := runCmd(dir, "bzr", "update", "-r", "revid:"+revid)
	return err
}

var validHgInfo = regexp.MustCompile(`^([a-f0-9]+) ([0-9]+)$`)

type hgVCS struct{}

func (hgVCS) Info(dir string) (VCSInfo, error) {
	out, err := runCmd(dir, "hg", "identify", "-n", "-i")
	if err != nil {
		return VCSInfo{}, err
	}
	m := validHgInfo.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return VCSInfo{}, fmt.Errorf("bzr revision-info has unexpected result %q", out)
	}
	// TODO(rog) check that tree is clean
	return VCSInfo{
		revid: m[1],
		revno: m[2],
	}, nil
}

func (hgVCS) Kind() string {
	return "hg"
}

func (hgVCS) Update(dir string, revid string) error {
	_, err := runCmd(dir, "hg", "update", revid)
	return err
}

func runCmd(dir string, name string, args ...string) (string, error) {
	var outData, errData bytes.Buffer
	c := exec.Command(name, args...)
	c.Stdout = &outData
	c.Stderr = &errData
	c.Dir = dir
	err := c.Run()
	if err == nil {
		return outData.String(), nil
	}
	if _, ok := err.(*exec.ExitError); ok && errData.Len() > 0 {
		return "", errors.New(strings.TrimSpace(errData.String()))
	}
	return "", fmt.Errorf("cannot run %q: %v", name, err)
}

var errorf = func(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "godeps: %s\n", fmt.Sprintf(f, a...))
	exitCode = 1
}
