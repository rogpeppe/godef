// The gosym command prints symbols in Go source code.
package main

import (
	"bufio"
	"bytes"
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/printer"
	"code.google.com/p/rog-go/exp/go/sym"
	"code.google.com/p/rog-go/exp/go/token"
	"code.google.com/p/rog-go/exp/go/types"
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TODO allow changing of package identifiers too.

// caveats:
// - no declaration for init
// - type switches?
// - embedded types
// - import to .
// - test files are ignored.
// - can't change package identifiers
// - there's no way to give an error if renaming creates a
//	clash of symbols.

//gosym list [-t] [pkg...]
//
//list all symbols in all named packages.
//	foo/filename.go:23:3: package referenced-package name type-kind
//
//gosym used pkg...
//
//	reads lines in long format; prints any definitions (in long format)
//	found in pkgs that are used by any other packages.
//
//gosym unused pkg
//	reads lines in long format; prints any definitions (in long format)
//	found in pkgs that are not used by any other packages.
//
//gosym unexport
//	reads lines in long or short format; changes any
//	identifier names to be uncapitalised.
//
//gosym short
//	reads lines in long or short format; prints them in short format.
//
//gosym rename from1 to1 from2 to2 ...
//	reads lines in long or short format; renames symbols according
//	to the given rules.
//
//gosym write [pkg...]
//	reads lines in short format; makes any requested changes,
//	restricting changes to the listed packages.
var verbose = new(bool)	// TODO!

func main() {
	printf := func(f string, a ...interface{}) { fmt.Fprintf(os.Stderr, f, a...) }
	flag.Usage = func() {
		printf("usage: gosym [flags] [pkgpath...]\n")
		flag.PrintDefaults()
		printf("%s", `
Gosym prints a line for each identifier used in the given
packages. Each line printed has at least 5 space-separated fields
in the following format:
	file-position package referenced-package name type-kind

The file-position field holds the location of the identifier.
The package field holds the path of the package containing the identifier.
The referenced-package field holds the path of the package
where the identifier is defined.
The name field holds the name of the identifier (in X.Y format if
it is defined as a member of another type X).
The type-kind field holds the type class of identifier (const,
type, var or func), and ends with a "+" sign if this line
marks the definition of the identifier.

When the -w flag is specified, gosym reads lines from its standard
symbols and changes symbols in the named packages accordingly. It
expects lines in the same format that it prints. Each identifier at the
line's file-position is changed to the name field.  If the type-kind
field ends with a "+" sign, all occurrences of the identifier will be
changed. Nothing will be changed outside the named packages.

As with gofix, writes are destructive - make sure your
source files are backed up before using gosym -w.
`)
		os.Exit(2)
	}
	if len(os.Args) < 2 {
		flag.Usage()
	}
	name := os.Args[1]
	var c cmd
	var args []string
	for _, e := range cmds {
		if e.name == name {
			e.fset.Parse(os.Args[2:])
			c = e.c
			args = e.fset.Args()
			break
		}
	}
	if c == nil {
		flag.Usage()
	}
	if err := runCmd(c, args); err != nil {
		log.Printf("gosym %s: %v", name, err)
		os.Exit(1)
	}
}

func runCmd(c cmd, args []string) error {
	types.Panic = false
	initGoPath()
	ctxt := newContext()
	defer ctxt.stdout.Flush()
	return c.run(ctxt, args)
}

type cmd interface {
	run(*context, []string) error
}

type cmdEntry struct {
	name string
	c cmd
	fset *flag.FlagSet
}

var cmds []cmdEntry

func register(name string, c cmd, fset *flag.FlagSet) {
	if fset == nil {
		fset = flag.NewFlagSet("gosym "+name, flag.ExitOnError)
	}
	cmds = append(cmds, cmdEntry{
		name: name,
		c: c,
		fset: fset,
	})
}
	
type context struct {
	mu sync.Mutex
	*sym.Context
	pkgCache map[string]*ast.Package
	pkgDirs  map[string]string // map from directory to package name.
	stdout   *bufio.Writer
}

func newContext() *context {
	ctxt := &context{
		pkgDirs: make(map[string]string),
		stdout:  bufio.NewWriter(os.Stdout),
		Context: sym.NewContext(),
	}
	ctxt.Logf = func(pos token.Pos, f string, a ...interface{}) {
		if !*verbose {
			return
		}
		log.Printf("%v: %s", ctxt.position(pos), fmt.Sprintf(f, a...))
	}
	return ctxt
}

func initGoPath() {
	// take GOPATH, set types.GoPath to it if it's not empty.
	p := os.Getenv("GOPATH")
	if p == "" {
		return
	}
	gopath := strings.Split(p, ":")
	for i, d := range gopath {
		gopath[i] = filepath.Join(d, "src")
	}
	r := os.Getenv("GOROOT")
	if r != "" {
		gopath = append(gopath, r+"/src/pkg")
	}
	types.GoPath = gopath
}

func (ctxt *context) positionToImportPath(p token.Position) string {
	if p.Filename == "" {
		panic("empty file name")
	}
	dir := filepath.Dir(p.Filename)
	if pkg, ok := ctxt.pkgDirs[dir]; ok {
		return pkg
	}
	bpkg, err := build.Import(".", dir, build.FindOnly)
	if err != nil {
		panic(fmt.Errorf("cannot reverse-map filename to package: %v", err))
	}
	ctxt.pkgDirs[dir] = bpkg.ImportPath
	return bpkg.ImportPath
}

func (ctxt *context) printf(f string, a ...interface{}) {
	fmt.Fprintf(ctxt.stdout, f, a...)
}

func (ctxt *context) position(pos token.Pos) token.Position {
	return ctxt.FileSet.Position(pos)
}

var emptyFileSet = token.NewFileSet()

func pretty(n ast.Node) string {
	var b bytes.Buffer
	printer.Fprint(&b, emptyFileSet, n)
	return b.String()
}
