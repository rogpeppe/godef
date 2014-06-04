package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/kisielk/gotool"
)

var (
	noTestDeps = flag.Bool("T", false, "exclude test dependencies")
	all        = flag.Bool("a", false, "show all dependencies recursively")
	std        = flag.Bool("stdlib", false, "show stdlib dependencies")
	from = flag.Bool("from", false, "show which dependencies are introduced by which packages")
)

var cwd string

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: showdeps [flags] pkg....\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	pkgs := flag.Args()
	if len(pkgs) == 0 {
		pkgs = []string{"."}
	}
	if d, err := os.Getwd(); err != nil {
		log.Fatalf("cannot get working directory: %v", err)
	} else {
		cwd = d
	}
	pkgs = gotool.ImportPaths(pkgs)
	allPkgs := make(map[string][]string)
	for _, pkg := range pkgs {
		if err := findImports(pkg, allPkgs); err != nil {
			log.Fatalf("cannot find imports from %q: %v", pkg, err)
		}
	}
	result := make([]string, 0, len(allPkgs))
	for name := range allPkgs {
		result = append(result, name)
	}
	sort.Strings(result)
	for _, r := range result {
		if *from {
			sort.Strings(allPkgs[r])
			fmt.Printf("%s %s\n", r, strings.Join(allPkgs[r], " "))
		} else {
			fmt.Println(r)
		}
	}
}

func isStdlib(pkg string) bool {
	return !strings.Contains(strings.SplitN(pkg, "/", 2)[0], ".")
}

// findImports recursively adds all imported packages of given
// package (packageName) to allPkgs map.
func findImports(packageName string, allPkgs map[string][]string) error {
	if packageName == "C" {
		return nil
	}
	pkg, err := build.Default.Import(packageName, cwd, 0)
	if err != nil {
		return fmt.Errorf("cannot find %q: %v", packageName, err)
	}
	for name := range imports(pkg) {
		if !*std && isStdlib(name) || name == pkg.ImportPath {
			continue
		}
		alreadyDone := allPkgs[name] != nil
		allPkgs[name] = append(allPkgs[name], pkg.ImportPath)
		if *all && !alreadyDone {
			if err := findImports(name, allPkgs); err != nil {
				return err
			}
		}
	}
	return nil
}

func addMap(m map[string]bool, ss []string) {
	for _, s := range ss {
		m[s] = true
	}
}

func imports(pkg *build.Package) map[string]bool {
	imps := make(map[string]bool)
	addMap(imps, pkg.Imports)
	if !*noTestDeps {
		addMap(imps, pkg.TestImports)
		addMap(imps, pkg.XTestImports)
	}
	return imps
}
