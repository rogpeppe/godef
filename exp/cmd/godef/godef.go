package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/printer"
	"io/ioutil"
	"os"
	"path/filepath"
	"rog-go.googlecode.com/hg/exp/go/parser"
	"rog-go.googlecode.com/hg/exp/go/types"
	"runtime"
	"sort"
	"strings"
)

var readStdin = flag.Bool("i", false, "read file from stdin")
var offset = flag.Int("o", -1, "file offset of identifier")
var debug = flag.Bool("debug", false, "debug mode")
var rflag = flag.Bool("r", false, "offset is specified in unicode code points instead of bytes")
var tflag = flag.Bool("t", false, "print type information")
var aflag = flag.Bool("a", false, "print type and member information")

func fail(s string, a ...interface{}) {
	fmt.Fprint(os.Stderr, "godef: "+fmt.Sprintf(s, a...)+"\n")
	os.Exit(2)
}

func init() {
	// take GOPATH, set types.GoPath to it if it's not empty.
	p := os.Getenv("GOPATH")
	if p == "" {
		return
	}
	gopath := strings.Split(p, ":")
	for i, d := range gopath {
		gopath[i] = filepath.Join(d, "src/pkg")
	}
	types.GoPath = gopath
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: godef [flags] file [expr]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 || flag.NArg() > 2 {
		flag.Usage()
		os.Exit(2)
	}
	types.Debug = *debug
	*tflag = *tflag || *aflag
	searchpos := *offset
	filename := flag.Arg(0)

	var src []byte
	if *readStdin {
		src, _ = ioutil.ReadAll(os.Stdin)
	} else {
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			fail("cannot read %s: %v", filename, err)
		}
		src = b
	}
	pkgScope := ast.NewScope(parser.Universe)
	f, err := parser.ParseFile(types.FileSet, filename, src, 0, pkgScope)
	if f == nil {
		fail("cannot parse %s: %v", filename, err)
	}

	var e ast.Expr
	switch {
	case flag.NArg() > 1:
		e = parseExpr(f.Scope, flag.Arg(1))

	case searchpos >= 0:
		if *rflag {
			searchpos = runeOffset2ByteOffset(src, searchpos)
		}
		e = findIdentifier(f, searchpos)

	default:
		fmt.Fprintf(os.Stderr, "no expression or offset specified\n")
		flag.Usage()
		os.Exit(2)
	}
	if !*tflag {
		// try local declarations only
		if obj, typ := types.ExprType(e, types.DefaultImporter); obj != nil {
			done(obj, typ)
		}
	}
	// add declarations from other files in the local package and try again
	pkg, _ := parseLocalPackage(filename, f, pkgScope)
	if pkg == nil && !*tflag {
		fmt.Printf("parseLocalPackage error: %v\n", err)
	}
	if obj, typ := types.ExprType(e, types.DefaultImporter); obj != nil {
		done(obj, typ)
	}
	fail("no declaration found for %v", pretty{e})
}

// findIdentifier looks for an identifier at byte-offset searchpos
// inside the parsed source represented by node.
// If it is part of a selector expression, it returns
// that expression rather than the identifier itself.
//
func findIdentifier(f *ast.File, searchpos int) ast.Expr {
	ec := make(chan ast.Expr)
	go func() {
		var visit FVisitor = func(n ast.Node) bool {
			var id *ast.Ident
			switch n := n.(type) {
			case *ast.Ident:
				id = n
			case *ast.SelectorExpr:
				id = n.Sel
			default:
				return true
			}

			pos := types.FileSet.Position(id.NamePos)
			if pos.Offset <= searchpos && pos.Offset+len(id.Name) >= searchpos {
				ec <- n.(ast.Expr)
				runtime.Goexit()
			}
			return true
		}
		ast.Walk(visit, f)
		ec <- nil
	}()
	ev := <-ec
	if ev == nil {
		fail("no identifier found")
	}
	return ev
}

type orderedObjects []*ast.Object

func (o orderedObjects) Less(i, j int) bool { return o[i].Name < o[j].Name }
func (o orderedObjects) Len() int           { return len(o) }
func (o orderedObjects) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }

func done(obj *ast.Object, typ types.Type) {
	pos := types.FileSet.Position(types.DeclPos(obj))
	if pos.Column > 0 {
		pos.Column--
	}
	fmt.Printf("%v\n", pos)
	if typ.Kind != ast.Bad {
		if *tflag {
			fmt.Printf("\t%s (%s)\n", strings.Replace(typeStr(obj, typ), "\n", "\n\t", -1), typ.Pkg)
		}
		if *aflag {
			var m orderedObjects
			for obj := range typ.Iter(types.DefaultImporter) {
				m = append(m, obj)
			}
			sort.Sort(m)
			for _, obj := range m {
				if typ.Pkg != "" && !ast.IsExported(obj.Name) {
					continue
				}
				fmt.Printf("\t%v\n", types.FileSet.Position(types.DeclPos(obj)))
				id := ast.NewIdent(obj.Name)
				id.Obj = obj
				_, mt := types.ExprType(id, types.DefaultImporter)
				fmt.Printf("\t\t%s\n", strings.Replace(typeStr(obj, mt), "\n", "\n\t\t", -1))
			}
		}
	}
	os.Exit(0)
}

func typeStr(obj *ast.Object, typ types.Type) string {
	switch typ.Kind {
	case ast.Fun, ast.Var:
		return fmt.Sprintf("%s %v", obj.Name, pretty{typ.Node})
	case ast.Pkg:
		return fmt.Sprintf("import (%s %s)", obj.Name, typ.Node.(*ast.ImportSpec).Path.Value)
	case ast.Con:
		return fmt.Sprintf("const %s %v", obj.Name, pretty{typ.Node})
	case ast.Lbl:
		return fmt.Sprintf("label %s", obj.Name)
	case ast.Typ:
		typ = typ.Underlying(false, types.DefaultImporter)
		return fmt.Sprintf("type %s %v", obj.Name, pretty{typ.Node})
	}
	return fmt.Sprintf("unknown %s %v\n", obj.Name, typ.Kind)
}


func parseExpr(s *ast.Scope, expr string) ast.Expr {
	n, err := parser.ParseExpr(types.FileSet, "<arg>", expr, s)
	if err != nil {
		fail("cannot parse expression: %v", err)
	}
	switch n := n.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		return n
	}
	fail("no identifier found in expression")
	return nil
}

type FVisitor func(n ast.Node) bool

func (f FVisitor) Visit(n ast.Node) ast.Visitor {
	if f(n) {
		return f
	}
	return nil
}

func runeOffset2ByteOffset(b []byte, off int) int {
	r := 0
	for i, _ := range string(b) {
		if r == off {
			return i
		}
		r++
	}
	return len(b)
}

var errNoPkgFiles = os.NewError("no more package files found")
// parseLocalPackage reads and parses all go files from the
// current directory that implement the same package name
// the principal source file, except the original source file
// itself, which will already have been parsed.
//
func parseLocalPackage(filename string, src *ast.File, pkgScope *ast.Scope) (*ast.Package, os.Error) {
	pkg := &ast.Package{src.Name.Name, pkgScope, nil, map[string]*ast.File{filename: src}}
	d, f := filepath.Split(filename)
	if d == "" {
		d = "./"
	}
	fd, err := os.Open(d)
	if err != nil {
		return nil, errNoPkgFiles
	}
	defer fd.Close()

	list, err := fd.Readdirnames(-1)
	if err != nil {
		return nil, errNoPkgFiles
	}

	for _, pf := range list {
		file := filepath.Join(d, pf)
		if !strings.HasSuffix(pf, ".go") ||
			pf == f ||
			pkgName(file) != pkg.Name {
			continue
		}
		src, err := parser.ParseFile(types.FileSet, file, nil, 0, pkg.Scope)
		if err == nil {
			pkg.Files[file] = src
		}
	}
	if len(pkg.Files) == 1 {
		return nil, errNoPkgFiles
	}
	return pkg, nil
}


// pkgName returns the package name implemented by the
// go source filename.
//
func pkgName(filename string) string {
	prog, _ := parser.ParseFile(types.FileSet, filename, nil, parser.PackageClauseOnly, nil)
	if prog != nil {
		return prog.Name.Name
	}
	return ""
}

func hasSuffix(s, suff string) bool {
	return len(s) >= len(suff) && s[len(s)-len(suff):] == suff
}

type pretty struct {
	n interface{}
}

func (p pretty) String() string {
	var b bytes.Buffer
	printer.Fprint(&b, types.FileSet, p.n)
	return b.String()
}
