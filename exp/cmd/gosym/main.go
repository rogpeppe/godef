// The gosym command manipulates symbols in Go source code.
// It supports the following commands:
//
// gosym short
// 
// The short command reads lines from standard input
// in short or long format (see the list command) and
// prints them in short format:
// 	file-position name new-name
// The file-position field holds the location of the identifier.
// The name field holds the name of the identifier (in X.Y format if
// it is defined as a member of another type X).
// The new-name field holds the desired new name for the identifier.
// 
// gosym export
// 
// The export command reads lines in short or long
// format from its standard input and capitalises the first letter
// of all symbols (thus making them available to external
// packages)
// 
// Note that this may cause clashes with other symbols
// that have already been defined with the new capitalisation.
// 
// gosym unexport
// 
// The unexport command reads lines in short or long
// format from its standard input and uncapitalises the first letter
// of all symbols (thus making them unavailable to external
// packages).
// 
// Note that this may cause clashes with other symbols
// that have already been defined with the new capitalisation.
// 
// gosym rename [old new]...
// 
// The rename command renames any symbol with the
// given old name to the given new name. The
// qualifier symbol's qualifier is ignored.
// 
// Note that this may cause clashes with other symbols
// that have already been defined with the new name.
// 
// gosym used pkg...
// 
// The used command reads lines in long format from the standard input and
// prints (in long format) any definitions found in the named packages that
// have references to them from any other package.
// 
// gosym unused pkg...
// 
// The unused command reads lines in long format from the standard input and
// prints (in long format) any definitions found in the named packages that
// have no references to them from any other package.
// 
// gosym list [flags] [pkg...]
// 
// The list command prints a line for each identifier
// used in the named packages. Each line printed has at least 6 space-separated fields
// in the following format:
// 	file-position referenced-file-position package referenced-package name type-kind
// This format is known as "long" format.
// If no packages are named, "." is used.
// 
// The file-position field holds the location of the identifier.
// The referenced-file-position field holds the location of the
// definition of the identifier.
// The package field holds the path of the package containing the identifier.
// The referenced-package field holds the path of the package
// where the identifier is defined.
// The name field holds the name of the identifier (in X.Y format if
// it is defined as a member of another type X).
// The type-kind field holds the type class of identifier (const,
// type, var or func), and ends with a "+" sign if this line
// marks the definition of the identifier.
//   -a=false: print internal and universe symbols too
//   -k="type,const,var,func": kinds of symbol types to include
//   -t=false: print symbol type
//   -v=false: print warnings about undefined symbols
// 
// gosym write [pkg...]
// 
// The gosym command reads lines in short format (see the
// "short" subcommand) from its standard input
// that represent changes to make, and changes any of the
// named packages accordingly - that is, the identifier
// at each line's file-position (and all uses of it) is changed to the new-name
// field.
// 
// If no packages are named, "." is used. No files outside the named packages
// will be changed. The names of any changed files will
// be printed.
// 
// As with gofix, writes are destructive - make sure your
// source files are backed up before using this command.
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

// CAVEATS:
// - map keys are not properly resolved.
// - no declaration for init
// - identifer in a type switch probably doesn't work properly.
// - type names embedded in structs or interfaces don't rename properly.
// - import to . is not supported.
// - test files are not dealt with properly.
// - can't change package identifiers
// - there's no way to give an error if renaming creates a
//	clash of symbols.
// - conditional compilation not supported

var verbose = flag.Bool("v", true, "print warning messages")

func main() {
	printf := func(f string, a ...interface{}) { fmt.Fprintf(os.Stderr, f, a...) }
	flag.Usage = func() {
		printf("usage: gosym [-v] command [flags] [args...]\n")
		printf("%s", `
Gosym manipulates symbols in Go source code.
Various sub-commands print, process or write symbols.
`)
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
	}
	name := flag.Arg(0)
	if name == "help" {
		help()
		return
	}
	var c cmd
	var args []string
	for _, e := range cmds {
		if e.name == name {
			e.fset.Parse(flag.Args()[1:])
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
	name  string
	about string
	c     cmd
	fset  *flag.FlagSet
}

var cmds []cmdEntry

func register(name string, c cmd, fset *flag.FlagSet, about string) {
	if fset == nil {
		fset = flag.NewFlagSet("gosym "+name, flag.ExitOnError)
	}
	fset.Usage = func() {
		fmt.Fprint(os.Stderr, about)
		fset.PrintDefaults()
	}
	cmds = append(cmds, cmdEntry{
		name:  name,
		about: about,
		c:     c,
		fset:  fset,
	})
}

func help() {
	for i, e := range cmds {
		if i > 0 {
			fmt.Printf("\n")
		}
		e.fset.SetOutput(os.Stdout)
		fmt.Print(e.about)
		e.fset.PrintDefaults()
	}
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
