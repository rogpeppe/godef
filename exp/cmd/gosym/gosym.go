// The gosym command prints symbols in Go source code.
package main

// caveats:
// - no declaration for init
// - type switches?
// - embedded types
// - import to .
// - test files.

import (
	"bufio"
	"bytes"
	"code.google.com/p/rog-go/exp/go/parser"
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/printer"
	"code.google.com/p/rog-go/exp/go/token"
	"code.google.com/p/rog-go/exp/go/types"
	"flag"
	"io"
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"regexp"
	"sync"
)

// TODO allow changing of package identifiers too.
var objKinds = map[string]ast.ObjKind{
	"const": ast.Con,
	"type":  ast.Typ,
	"var":   ast.Var,
	"func":  ast.Fun,
}

var (
	verbose = flag.Bool("v", false, "print warnings for unresolved symbols")
	kinds   = flag.String("k", allKinds(), "kinds of symbol types to include")
	printType = flag.Bool("t", false, "print symbol type")
	all = flag.Bool("a", false, "print internal and universe symbols too")
	wflag = flag.Bool("w", false, "read lines; change symbols in source code")
)

func main() {
	printf := func(f string, a ...interface{}) { fmt.Fprintf(os.Stderr, f, a...) }
	flag.Usage = func() {
		printf("usage: gosym [flags] [pkgpath...]\n")
		flag.PrintDefaults()
		printf("Each line printed has the following format:\n")
		printf("file-position package referenced-package type-name type-kind\n")
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
	ctxt *context
	// lines holds all input lines.
	lines map[token.Position] symLine
	// plusPkgs holds packages that have a line with a "+"
	plusPkgs map[string] bool
	// symPkgs holds all packages mentioned in the input lines.
	symPkgs map[string]bool
}

func writeSyms(ctxt *context, pkgs []string) error {
	wctxt := &wcontext{
		ctxt: ctxt,
		lines: make(map[token.Position] symLine),
		plusPkgs: make(map[string]bool),
		symPkgs: make(map[string]bool),
	}
	if err := wctxt.readSymbols(os.Stdin); err != nil {
		return fmt.Errorf("failed to read symbols: %v", err)
	}

	// Search for all symbols that need replacing globally.
	for pkg := range wctxt.plusPkgs {
		ctxt.printf("searching in %v\n", pkg)
	}
	return nil
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
		pkg := wctxt.ctxt.positionToImportPath(sl.pos)
		if sl.plus {
			wctxt.plusPkgs[pkg] = true
		}
		wctxt.symPkgs[pkg] = true
	}
	return nil
}

func printSyms(ctxt *context, mask uint, pkgs []string) {
	visitor := func(info *symInfo) bool {
		return visitPrint(ctxt, info, mask)
	}
	types.Panic = false
	for _, path := range pkgs {
		if pkg := ctxt.importer(path); pkg != nil {
			for _, f := range pkg.Files {
				ctxt.visitExprs(visitor, path, f, mask)
			}
		}
	}
}

type context struct {
	mu sync.Mutex
	pkgCache map[string]*ast.Package
	pkgDirs map[string]string		// map from directory to package name.
	importer func(path string) *ast.Package
	stdout *bufio.Writer
}

func newContext() *context {
	ctxt := &context {
		pkgCache: make(map[string]*ast.Package),
		pkgDirs: make(map[string]string),
		stdout: bufio.NewWriter(os.Stdout),
	}
	ctxt.importer =  func(path string) *ast.Package {
		ctxt.mu.Lock()
		defer ctxt.mu.Unlock()
		if pkg := ctxt.pkgCache[path]; pkg != nil {
			return pkg
		}
		pkg := types.DefaultImporter(path)
		ctxt.pkgCache[path] = pkg
		return pkg
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

type astVisitor func(n ast.Node) bool

func (f astVisitor) Visit(n ast.Node) ast.Visitor {
	if f(n) {
		return f
	}
	return nil
}

func (ctxt *context) visitExprs(visitf func(*symInfo) bool, importPath string, pkg *ast.File, kindMask uint) {
	var visit astVisitor
	ok := true
	local := false		// TODO set to true inside function body
	visit = func(n ast.Node) bool {
		if !ok {
			return false
		}
		switch n := n.(type) {
		case *ast.ImportSpec:
			// If the file imports a package to ".", abort
			// because we don't support that (yet).
			if n.Name != nil && n.Name.Name == "." {
				log.Printf("import to . not supported")
				ok = false
				return false
			}
			return true

		case *ast.FuncDecl:
			// add object for init functions
			if n.Recv == nil && n.Name.Name == "init" {
				n.Name.Obj = ast.NewObj(ast.Fun, "init")
			}
			return true

		case *ast.Ident:
			ok = ctxt.visitExpr(visitf, importPath, n, local)
			return false

		case *ast.KeyValueExpr:
			// don't try to resolve the key part of a key-value
			// because it might be a map key which doesn't
			// need resolving, and we can't tell without being
			// complicated with types.
			ast.Walk(visit, n.Value)
			return false

		case *ast.SelectorExpr:
			ast.Walk(visit, n.X)
			ok = ctxt.visitExpr(visitf, importPath, n, local)
			return false

		case *ast.File:
			for _, d := range n.Decls {
				ast.Walk(visit, d)
			}
			return false
		}

		return true
	}
	ast.Walk(visit, pkg)
}

type symInfo struct {
	pos token.Pos			// position of symbol.
	expr ast.Expr			// expression for symbol (*ast.Ident or *ast.SelectorExpr)
	exprType types.Type	// type of expression.
	referPos token.Pos		// position of referred-to symbol.
	referObj *ast.Object		// object referred to. 
	local bool				// whether referred-to object is function-local.
	universe bool			// whether referred-to object is in universe.
}

func (ctxt *context) visitExpr(visitf func(*symInfo) bool, importPath string, e ast.Expr, local bool) bool {
	var info symInfo
	info.expr = e
	switch e := e.(type) {
	case *ast.Ident:
		info.pos = e.Pos()
	case *ast.SelectorExpr:
		info.pos = e.Sel.Pos()
	}
	obj, t := types.ExprType(e, ctxt.importer)
	if obj == nil {
		if *verbose {
			log.Printf("%v: no object for %s", position(e.Pos()), pretty{e})
		}
		return true
	}
	info.exprType = t
	info.referObj = obj
	if parser.Universe.Lookup(obj.Name) != obj {
		info.referPos = types.DeclPos(obj)
		if info.referPos == token.NoPos {
			log.Printf("%v: no declaration for %s", position(e.Pos()), pretty{e})
			return true
		}
	} else {
		info.universe = true
	}
	info.local = local
	return visitf(&info)
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

type symLine struct {
	pos token.Position	// file address of identifier; addr.Offset is zero.
	exprPkg string		// package containing identifier
	referPkg string		// package containing referred-to object.
	local bool			// identifier is function-local
	kind ast.ObjKind		// kind of identifier
	plus bool		// line is, or refers to, definition of object.
	expr string		// expression.
	exprType string	// type of expression (unparsed).
}

var linePat = regexp.MustCompile(`^([^:]+):(\d+):(\d+):\s+([^ ]+)\s+([^\s]+)\s+([^\s]+)\s+(local)?([^\s+]+)(\+)?(\s+([^\s].*))?$`)

func atoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic("bad number")
	}
	return i
}

func parseSymLine(line string) (symLine, error) {
	m := linePat.FindStringSubmatch(line)
	if m == nil {
		return symLine{}, fmt.Errorf("invalid line %q", line)
	}
	var l symLine
	l.pos.Filename = m[1]
	l.pos.Line = atoi(m[2])
	l.pos.Column = atoi(m[3])
	l.exprPkg = m[4]
	l.referPkg = m[5]
	l.expr = m[6]		// TODO check for invalid chars in expr
	l.local = m[7] == "local"
	var ok bool
	l.kind, ok = objKinds[m[8]]
	if !ok {
		return symLine{}, fmt.Errorf("invalid kind %q", m[8])
	}
	l.plus = m[9] == "+"
	if m[10] != "" {
		l.exprType = m[11]
	}
	return l, nil
}

func (l symLine) String() string {
	local := ""
	if l.local {
		local = "local"
	}
	def := ""
	if l.plus {
		def = "+"
	}
	exprType := ""
	if len(l.exprType) > 0 {
		exprType = " " + l.exprType
	}
	return fmt.Sprintf("%v: %s %s %s %s%s%s%s", l.pos, l.exprPkg, l.referPkg, l.expr, local, l.kind, def, exprType)
}

func visitPrint(ctxt *context, info *symInfo, kindMask uint) bool {
	if (1<<uint(info.referObj.Kind))&kindMask == 0 {
		return true
	}
	if info.universe && !*all {
		return true
	}
	eposition := position(info.pos)
	exprPkg := ctxt.positionToImportPath(eposition)
	var referPkg string
	if info.universe {
		referPkg = "universe"
	} else {
		referPkg = ctxt.positionToImportPath(position(info.referPos))
	}
	var name string
	switch e := info.expr.(type) {
	case *ast.Ident:
		name = e.Name
	case *ast.SelectorExpr:
		_, xt := types.ExprType(e.X, ctxt.importer)
		if xt.Node == nil {
			if *verbose {
				log.Printf("%v: no type for %s", position(e.Pos()), pretty{e.X})
				return true
			}
		}
		name = e.Sel.Name
		if xt.Kind != ast.Pkg {
			name = (pretty{depointer(xt.Node)}).String() + "." + name
		}
	}
	line := symLine{
		pos: eposition,
		exprPkg: exprPkg,
		referPkg: referPkg,
		local: info.local,
		kind: info.referObj.Kind,
		plus: info.referPos == info.pos,
		expr: name,
	}
	if *printType {
		line.exprType = (pretty{info.exprType.Node}).String()
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

// litToString converts from a string literal to a regular string.
func litToString(lit *ast.BasicLit) (v string) {
	if lit.Kind != token.STRING {
		panic("expected string")
	}
	v, err := strconv.Unquote(string(lit.Value))
	if err != nil {
		panic("cannot unquote")
	}
	return v
}

func position(pos token.Pos) token.Position {
	return types.FileSet.Position(pos)
}

type pretty struct {
	n interface{}
}

func (p pretty) String() string {
	var b bytes.Buffer
	printer.Fprint(&b, types.FileSet, p.n)
	return b.String()
}
