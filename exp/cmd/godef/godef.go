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
var bflag = flag.Bool("b", false, "offset is specified in bytes instead of code points")
var tflag = flag.Bool("t", false, "print type information")
var aflag = flag.Bool("a", false, "print type and member information")

func fail(s string, a ...interface{}) {
	fmt.Fprint(os.Stderr, "godef: "+fmt.Sprintf(s, a...)+"\n")
	os.Exit(2)
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
		if !*bflag {
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
		if obj, typ := types.ExprType(e); obj != nil {
			done(obj, typ)
		}
	}
	// add declarations from other files in the local package and try again
	pkg, err := parseLocalPackage(filename, f, pkgScope)
	if pkg == nil {
		fail("no declaration found for %v", pretty{e})
	}
	if obj, typ := types.ExprType(e); obj != nil {
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

func done(obj *ast.Object, typ types.Type) {
	pos := types.FileSet.Position(obj.Pos())
	if pos.Column > 0 {
		pos.Column--
	}
	fmt.Printf("%v\n", pos)
	if typ.Kind != ast.Bad {
		if *tflag {
			fmt.Printf("\t%s\n", strings.Replace(typeStr(obj, typ), "\n", "\n\t", -1))
		}
		if *aflag {
			var m []string
			for obj := range typ.Iter() {
				id := ast.NewIdent(obj.Name)
				id.Obj = obj
				_, mt := types.ExprInfo(id)
				m = append(m, strings.Replace(typeStr(obj, mt), "\n", "\n\t\t", -1))
			}
			sort.SortStrings(m)
			for _, s := range m {
				fmt.Printf("\t\t%s\n", s)
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
		typ = typ.Underlying(false)
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

var errNoPkgFiles = os.ErrorString("no more package files found")
// parseLocalPackage reads and parses all go files from the
// current directory that implement the same package name
// the principal source file, except the original source file
// itself, which will already have been parsed.
//
func parseLocalPackage(filename string, src *ast.File, pkgScope *ast.Scope) (*ast.Package, os.Error) {
	pkg := &ast.Package{src.Name.Name, pkgScope, map[string]*ast.File{filename: src}}
	d, f := filepath.Split(filename)
	if d == "" {
		d = "./"
	}
	fd, err := os.Open(d, os.O_RDONLY, 0)
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
		src, err := parser.ParseFile(types.FileSet, file, nil, parser.Declarations, pkg.Scope)
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
