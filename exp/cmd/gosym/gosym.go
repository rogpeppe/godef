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
	"io"
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

var objKinds = map[string]ast.ObjKind{
	"const": ast.Con,
	"type":  ast.Typ,
	"var":   ast.Var,
	"func":  ast.Fun,
}

var (
	verbose   = flag.Bool("v", false, "print warnings for unresolved symbols")
	kinds     = flag.String("k", allKinds(), "kinds of symbol types to include")
	printType = flag.Bool("t", false, "print symbol type")
	all       = flag.Bool("a", false, "print internal and universe symbols too")
	wflag     = flag.Bool("w", false, "read lines; change symbols in source code")
)

//gosym list [-l] [-t] [pkg...]
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
//gosym export
//	reads lines in long or short format; changes any
//	identifier names to be capitalised.
//	Prints identifiers in short format.
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
	flag.Parse()
	if *kinds == "" {
		flag.Usage()
	}
	pkgs := flag.Args()
	if len(pkgs) == 0 {
		pkgs = []string{"."}
	}
	mask, err := parseKindMask(*kinds)
	if err != nil {
		printf("gosym: %v", err)
		flag.Usage()
	}
	initGoPath()
	ctxt := newContext()
	defer ctxt.stdout.Flush()
	if *wflag {
		writeSyms(ctxt, pkgs)
	} else {
		printSyms(ctxt, mask, pkgs)
	}
}

type wcontext struct {
	*context

	// lines holds all input lines.
	lines map[token.Position]*symLine

	// plusPkgs holds packages that have a line with a "+"
	plusPkgs map[string]bool

	// symPkgs holds all packages mentioned in the input lines.
	symPkgs map[string]bool

	// globalReplace holds all the objects that
	// will be globally replaced and the new name
	// of the object's symbol.
	globalReplace map[*ast.Object]string

	// changed holds all the files that have been modified.
	changed map[*ast.File]bool
}

func writeSyms(ctxt *context, pkgs []string) error {
	wctxt := &wcontext{
		context:       ctxt,
		lines:         make(map[token.Position]*symLine),
		plusPkgs:      make(map[string]bool),
		symPkgs:       make(map[string]bool),
		globalReplace: make(map[*ast.Object]string),
	}
	if err := wctxt.readSymbols(os.Stdin); err != nil {
		return fmt.Errorf("failed to read symbols: %v", err)
	}
	wctxt.addGlobals()
	wctxt.replace(pkgs)
	for name := range wctxt.ChangedFiles {
		wctxt.printf("%s\n", name)
	}
	if err := wctxt.WriteFiles(wctxt.ChangedFiles); err != nil {
		return err
	}
	return nil
}

// replace replaces all symbols in files as directed by
// the input lines.
func (wctxt *wcontext) replace(pkgs []string) {
	visitor := func(info *sym.Info) bool {
		globSym, globRepl := wctxt.globalReplace[info.ReferObj]
		p := wctxt.position(info.Pos)
		p.Offset = 0
		line, lineRepl := wctxt.lines[p]
		if !lineRepl && !globRepl {
			return true
		}
		var newSym string
		if lineRepl {
			if newSym = line.symName(); newSym == info.ReferObj.Name {
				// There is a line for this symbol, but the name is
				// not changing, so ignore it.
				lineRepl = false
			}
		}
		if globRepl {
			// N.B. global symbols are not recorded in globalReplace
			// if they make no change.
			if lineRepl && globSym != newSym {
				log.Printf("gosym: %v: conflicting global/local change (%q vs %q)", p, globSym, newSym)
				return true
			}
			newSym = globSym
		}
		info.Ident.Name = newSym
		return true
	}
	for _, path := range pkgs {
		pkg := wctxt.Import(path)
		if pkg == nil {
			log.Printf("gosym: could not find package %q", path)
			continue
		}
		for _, f := range pkg.Files {
			// TODO when no global replacements, don't bother if file
			// isn't mentioned in input lines.
			wctxt.IterateSyms(f, visitor)
		}
	}
}

func (wctxt *wcontext) addGlobals() {
	// visitor adds a symbol to wctxt.globalReplace if necessary.
	visitor := func(info *sym.Info) bool {
		p := wctxt.position(info.Pos)
		p.Offset = 0
		line, ok := wctxt.lines[p]
		if !ok || !line.plus {
			return true
		}
		sym := line.symName()
		if info.ReferObj.Name == sym {
			// If the symbol name is not being changed, do nothing.
			return true
		}
		if old, ok := wctxt.globalReplace[info.ReferObj]; ok {
			if old != sym {
				log.Printf("gosym: %v: conflicting replacement for %s", p, line.expr)
				return true
			}
		}
		wctxt.globalReplace[info.ReferObj] = line.symName()
		return true
	}

	// Search for all symbols that need replacing globally.
	for path := range wctxt.plusPkgs {
		pkg := wctxt.Import(path)
		if pkg == nil {
			log.Printf("gosym: could not find package %q", path)
			continue
		}
		for _, f := range pkg.Files {
			// TODO don't bother if file isn't mentioned in input lines.
			wctxt.IterateSyms(f, visitor)
		}
	}
}

// readSymbols reads all the symbols from stdin.
func (wctxt *wcontext) readSymbols(stdin io.Reader) error {
	r := bufio.NewReader(stdin)
	for {
		line, isPrefix, err := r.ReadLine()
		if err != nil {
			break
		}
		if isPrefix {
			log.Printf("line too long")
			break
		}
		sl, err := parseSymLine(string(line))
		if err != nil {
			log.Printf("cannot parse line %q: %v", line, err)
			continue
		}
		if old, ok := wctxt.lines[sl.pos]; ok {
			log.Printf("%v: duplicate symbol location; original at %v", sl.pos, old.pos)
			continue
		}
		wctxt.lines[sl.pos] = sl
		pkg := wctxt.positionToImportPath(sl.pos)
		if sl.plus {
			wctxt.plusPkgs[pkg] = true
		}
		wctxt.symPkgs[pkg] = true
	}
	return nil
}

func printSyms(ctxt *context, mask uint, pkgs []string) {
	visitor := func(info *sym.Info) bool {
		return visitPrint(ctxt, info, mask)
	}
	types.Panic = false
	for _, path := range pkgs {
		if pkg := ctxt.Import(path); pkg != nil {
			for _, f := range pkg.Files {
				ctxt.IterateSyms(f, visitor)
			}
		}
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

func parseKindMask(kinds string) (uint, error) {
	mask := uint(0)
	ks := strings.Split(kinds, ",")
	for _, k := range ks {
		c, ok := objKinds[k]
		if ok {
			mask |= 1 << uint(c)
		} else {
			return 0, fmt.Errorf("unknown type kind %q", k)
		}
	}
	return mask, nil
}

func allKinds() string {
	var ks []string
	for k := range objKinds {
		ks = append(ks, k)
	}
	return strings.Join(ks, ",")
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

func visitPrint(ctxt *context, info *sym.Info, kindMask uint) bool {
	if (1<<uint(info.ReferObj.Kind))&kindMask == 0 {
		return true
	}
	if info.Universe && !*all {
		return true
	}
	eposition := ctxt.position(info.Pos)
	exprPkg := ctxt.positionToImportPath(eposition)
	var referPkg string
	if info.Universe {
		referPkg = "universe"
	} else {
		referPkg = ctxt.positionToImportPath(ctxt.position(info.ReferPos))
	}
	var name string
	switch e := info.Expr.(type) {
	case *ast.Ident:
		name = e.Name
	case *ast.SelectorExpr:
		_, xt := types.ExprType(e.X, func(path string) *ast.Package {
			return ctxt.Import(path)
		})
		if xt.Node == nil {
			if *verbose {
				log.Printf("%v: no type for %s", ctxt.position(e.Pos()), pretty(e.X))
				return true
			}
		}
		name = e.Sel.Name
		if xt.Kind != ast.Pkg {
			name = pretty(depointer(xt.Node)) + "." + name
		}
	}
	line := &symLine{
		long:     true,
		pos:      eposition,
		exprPkg:  exprPkg,
		referPkg: referPkg,
		local:    info.Local,
		kind:     info.ReferObj.Kind,
		plus:     info.ReferPos == info.Pos,
		expr:     name,
	}
	if *printType {
		line.exprType = pretty(info.ExprType.Node)
	}
	ctxt.printf("%s\n", line)
	return true
}

func depointer(x ast.Node) ast.Node {
	if x, ok := x.(*ast.StarExpr); ok {
		return x.X
	}
	return x
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
