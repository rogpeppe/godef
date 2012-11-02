package main

import (
	"bytes"
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/printer"
	"code.google.com/p/rog-go/exp/go/token"
	"code.google.com/p/rog-go/exp/go/types"
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
)

func main() {
	printf := func(f string, a ...interface{}) { fmt.Fprintf(os.Stderr, f, a...) }
	flag.Usage = func() {
		printf("usage: gosym [flags] pkgpath...\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() < 1 || *kinds == "" {
		flag.Usage()
	}
	pkgs := flag.Args()
	mask, err := parseKindMask(*kinds)
	if err != nil {
		printf("gosym: %v", err)
		flag.Usage()
	}
	initGoPath()

	cache := make(map[string]*ast.Package)
	importer := func(path string) *ast.Package {
		if pkg := cache[path]; pkg != nil {
			return pkg
		}
		pkg := types.DefaultImporter(path)
		cache[path] = pkg
		return pkg
	}

	types.Panic = false
	for _, path := range pkgs {
		if pkg := importer(path); pkg != nil {
			for _, f := range pkg.Files {
				checkExprs(path, f, importer, mask)
			}
		}
	}
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

func checkExprs(importPath string, pkg *ast.File, importer types.Importer, kindMask uint) {
	var visit astVisitor
	stopped := false
	visit = func(n ast.Node) bool {
		if stopped {
			return false
		}
		switch n := n.(type) {
		case *ast.ImportSpec:
			// If the file imports a package to ".", abort
			// because we don't support that (yet).
			if n.Name != nil && n.Name.Name == "." {
				stopped = true
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
			printSelector(importPath, n, importer, kindMask)
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

func printSelector(importPath string, e *ast.SelectorExpr, importer types.Importer, kindMask uint) {
	_, xt := types.ExprType(e.X, importer)
	if xt.Node == nil {
		if *verbose {
			log.Printf("%v: no type for %s", position(e.Pos()), pretty{e.X})
		}
	}
	obj, t := types.ExprType(e, importer)
	if obj == nil {
		if *verbose {
			log.Printf("%v: no object for %s", position(e.Pos()), pretty{e})
		}
		return
	}

	// Exclude any type kinds not in the user-specified mask.
	if (1<<uint(obj.Kind))&kindMask == 0 {
		return
	}

	var pkgPath, xexpr string
	if xt.Kind == ast.Pkg {
		// TODO make the Node of the package its identifier.
		// and put the package name in xt.Pkg.
		pkgPath = litToString(xt.Node.(*ast.ImportSpec).Path)
		xexpr = ""
	} else {
		pos := position(types.DeclPos(obj))
		if pos.Filename == "" {
			panic("empty file name")
		}
		pkgPath = dirToImportPath(filepath.Dir(pos.Filename))
		xexpr = (pretty{depointer(xt.Node)}).String() + "."
	}
	typeStr := ""
	if *printType {
		typeStr = " " + (pretty{t.Node}).String()
	}
	fmt.Printf("%v: %s %s %s%s %s%s\n", position(e.Pos()), importPath, pkgPath, xexpr, e.Sel.Name, obj.Kind, typeStr)
}

func dirToImportPath(dir string) string {
	bpkg, err := build.Import(".", dir, build.FindOnly)
	if err != nil {
		panic(fmt.Errorf("cannot reverse-map filename to package: %v", err))
	}
	return bpkg.ImportPath
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
