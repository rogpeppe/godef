package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	debugpkg "runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"

	"github.com/rogpeppe/godef/go/ast"
	"github.com/rogpeppe/godef/go/parser"
	"github.com/rogpeppe/godef/go/token"
	"github.com/rogpeppe/godef/go/types"
	"golang.org/x/tools/go/packages"
)

var readStdin = flag.Bool("i", false, "read file from stdin")
var offset = flag.Int("o", -1, "file offset of identifier in stdin")
var debug = flag.Bool("debug", false, "debug mode")
var tflag = flag.Bool("t", false, "print type information")
var aflag = flag.Bool("a", false, "print public type and member information")
var Aflag = flag.Bool("A", false, "print all type and members information")
var fflag = flag.String("f", "", "Go source filename")
var acmeFlag = flag.Bool("acme", false, "use current acme window")
var jsonFlag = flag.Bool("json", false, "output location in JSON format (-t flag is ignored)")

var cpuprofile = flag.String("cpuprofile", "", "write CPU profile to this file")
var memprofile = flag.String("memprofile", "", "write memory profile to this file")
var traceFlag = flag.String("trace", "", "write trace log to this file")

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "godef: %v\n", err)
		os.Exit(2)
	}
}

func run(ctx context.Context) error {
	// for most godef invocations we want to produce the result and quit without
	// ever triggering the GC, but we don't want to outright disable it for the
	// rare case when we are asked to handle a truly huge data set, so we set it
	// to a very large ratio. This number was picked to be significantly bigger
	// than needed to prevent GC on a common very large build, but is essentially
	// a magic number not a calculated one
	debugpkg.SetGCPercent(1600)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: godef [flags] [expr]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			return err
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}
		defer pprof.StopCPUProfile()
	}

	if *traceFlag != "" {
		f, err := os.Create(*traceFlag)
		if err != nil {
			return err
		}
		if err := trace.Start(f); err != nil {
			return err
		}
		defer func() {
			trace.Stop()
			log.Printf("To view the trace, run:\n$ go tool trace view %s", *traceFlag)
		}()
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			return err
		}
		defer func() {
			runtime.GC() // get up-to-date statistics
			if err := pprof.WriteHeapProfile(f); err != nil {
				log.Printf("Writing memory profile: %v", err)
			}
			f.Close()
		}()
	}

	types.Debug = *debug
	*tflag = *tflag || *aflag || *Aflag
	searchpos := *offset
	filename := *fflag

	var afile *acmeFile
	var src []byte
	if *acmeFlag {
		var err error
		if afile, err = acmeCurrentFile(); err != nil {
			return fmt.Errorf("%v", err)
		}
		filename, src, searchpos = afile.name, afile.body, afile.offset
	} else if *readStdin {
		src, _ = ioutil.ReadAll(os.Stdin)
	} else {
		// TODO if there's no filename, look in the current
		// directory and do something plausible.
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("cannot read %s: %v", filename, err)
		}
		src = b
	}
	// Load, parse, and type-check the packages named on the command line.
	cfg := &packages.Config{
		Context: ctx,
		Tests:   strings.HasSuffix(filename, "_test.go"),
	}
	obj, err := adaptGodef(cfg, filename, src, searchpos)
	if err != nil {
		return err
	}

	// print old source location to facilitate backtracking
	if *acmeFlag {
		fmt.Printf("\t%s:#%d\n", afile.name, afile.runeOffset)
	}

	return print(os.Stdout, obj)
}

func godef(filename string, src []byte, searchpos int) (*ast.Object, types.Type, error) {
	pkgScope := ast.NewScope(parser.Universe)
	f, err := parser.ParseFile(types.FileSet, filename, src, 0, pkgScope, types.DefaultImportPathToName)
	if f == nil {
		return nil, types.Type{}, fmt.Errorf("cannot parse %s: %v", filename, err)
	}

	var o ast.Node
	switch {
	case flag.NArg() > 0:
		o, err = parseExpr(f.Scope, flag.Arg(0))
		if err != nil {
			return nil, types.Type{}, err
		}

	case searchpos >= 0:
		o, err = findIdentifier(f, searchpos)
		if err != nil {
			return nil, types.Type{}, err
		}

	default:
		return nil, types.Type{}, fmt.Errorf("no expression or offset specified")
	}
	switch e := o.(type) {
	case *ast.ImportSpec:
		path, err := importPath(e)
		if err != nil {
			return nil, types.Type{}, err
		}
		pkg, err := build.Default.Import(path, filepath.Dir(filename), build.FindOnly)
		if err != nil {
			return nil, types.Type{}, fmt.Errorf("error finding import path for %s: %s", path, err)
		}
		return &ast.Object{Kind: ast.Pkg, Data: pkg.Dir}, types.Type{}, nil
	case ast.Expr:
		if !*tflag {
			// try local declarations only
			if obj, typ := types.ExprType(e, types.DefaultImporter, types.FileSet); obj != nil {
				return obj, typ, nil
			}
		}
		// add declarations from other files in the local package and try again
		pkg, err := parseLocalPackage(filename, f, pkgScope, types.DefaultImportPathToName)
		if pkg == nil && !*tflag {
			fmt.Printf("parseLocalPackage error: %v\n", err)
		}
		if flag.NArg() > 0 {
			// Reading declarations in other files might have
			// resolved the original expression.
			e, err = parseExpr(f.Scope, flag.Arg(0))
			if err != nil {
				return nil, types.Type{}, err
			}
		}
		if obj, typ := types.ExprType(e, types.DefaultImporter, types.FileSet); obj != nil {
			return obj, typ, nil
		}
		return nil, types.Type{}, fmt.Errorf("no declaration found for %v", pretty{e})
	}
	return nil, types.Type{}, nil
}

func importPath(n *ast.ImportSpec) (string, error) {
	p, err := strconv.Unquote(n.Path.Value)
	if err != nil {
		return "", fmt.Errorf("invalid string literal %q in ast.ImportSpec", n.Path.Value)
	}
	return p, nil
}

type nodeResult struct {
	node ast.Node
	err  error
}

// findIdentifier looks for an identifier at byte-offset searchpos
// inside the parsed source represented by node.
// If it is part of a selector expression, it returns
// that expression rather than the identifier itself.
//
// As a special case, if it finds an import
// spec, it returns ImportSpec.
//
func findIdentifier(f *ast.File, searchpos int) (ast.Node, error) {
	ec := make(chan nodeResult)
	found := func(startPos, endPos token.Pos) bool {
		start := types.FileSet.Position(startPos).Offset
		end := start + int(endPos-startPos)
		return start <= searchpos && searchpos <= end
	}
	go func() {
		var visit func(ast.Node) bool
		visit = func(n ast.Node) bool {
			var startPos token.Pos
			switch n := n.(type) {
			default:
				return true
			case *ast.Ident:
				startPos = n.NamePos
			case *ast.SelectorExpr:
				startPos = n.Sel.NamePos
			case *ast.ImportSpec:
				startPos = n.Pos()
			case *ast.StructType:
				// If we find an anonymous bare field in a
				// struct type, its definition points to itself,
				// but we actually want to go elsewhere,
				// so assume (dubiously) that the expression
				// works globally and return a new node for it.
				for _, field := range n.Fields.List {
					if field.Names != nil {
						continue
					}
					t := field.Type
					if pt, ok := field.Type.(*ast.StarExpr); ok {
						t = pt.X
					}
					if id, ok := t.(*ast.Ident); ok {
						if found(id.NamePos, id.End()) {
							expr, err := parseExpr(f.Scope, id.Name)
							ec <- nodeResult{expr, err}
							runtime.Goexit()
						}
					}
				}
				return true
			}
			if found(startPos, n.End()) {
				ec <- nodeResult{n, nil}
				runtime.Goexit()
			}
			return true
		}
		ast.Walk(FVisitor(visit), f)
		ec <- nodeResult{nil, nil}
	}()
	ev := <-ec
	if ev.err != nil {
		return nil, ev.err
	}
	if ev.node == nil {
		return nil, fmt.Errorf("no identifier found")
	}
	return ev.node, nil
}

type Position struct {
	Filename string `json:"filename,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

type Kind string

const (
	BadKind    Kind = "bad"
	FuncKind   Kind = "func"
	VarKind    Kind = "var"
	ImportKind Kind = "import"
	ConstKind  Kind = "const"
	LabelKind  Kind = "label"
	TypeKind   Kind = "type"
	PathKind   Kind = "path"
)

type Object struct {
	Name     string
	Kind     Kind
	Pkg      string
	Position Position
	Members  []*Object
	Type     interface{}
	Value    interface{}
}

type orderedObjects []*Object

func (o orderedObjects) Less(i, j int) bool { return o[i].Name < o[j].Name }
func (o orderedObjects) Len() int           { return len(o) }
func (o orderedObjects) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }

func print(out io.Writer, obj *Object) error {
	if obj.Kind == PathKind {
		fmt.Fprintf(out, "%s\n", obj.Value)
		return nil
	}
	if *jsonFlag {
		jsonStr, err := json.Marshal(obj.Position)
		if err != nil {
			return fmt.Errorf("JSON marshal error: %v", err)
		}
		fmt.Fprintf(out, "%s\n", jsonStr)
		return nil
	} else {
		fmt.Fprintf(out, "%v\n", obj.Position)
	}
	if obj.Kind == BadKind || !*tflag {
		return nil
	}
	fmt.Fprintf(out, "%s\n", typeStr(obj))
	if *aflag || *Aflag {
		for _, obj := range obj.Members {
			// Ignore unexported members unless Aflag is set.
			if !*Aflag && (obj.Pkg != "" || !ast.IsExported(obj.Name)) {
				continue
			}
			fmt.Fprintf(out, "\t%s\n", strings.Replace(typeStr(obj), "\n", "\n\t\t", -1))
			fmt.Fprintf(out, "\t\t%v\n", obj.Position)
		}
	}
	return nil
}

func typeStr(obj *Object) string {
	buf := &bytes.Buffer{}
	valueFmt := " = %v"
	switch obj.Kind {
	case VarKind, FuncKind:
		// don't print these
	case ImportKind:
		valueFmt = " %v)"
		fmt.Fprint(buf, obj.Kind)
		fmt.Fprint(buf, " (")
	default:
		fmt.Fprint(buf, obj.Kind)
		fmt.Fprint(buf, " ")
	}
	fmt.Fprint(buf, obj.Name)
	if obj.Type != nil {
		fmt.Fprintf(buf, " %v", pretty{obj.Type})
	}
	if obj.Value != nil {
		fmt.Fprintf(buf, valueFmt, pretty{obj.Value})
	}
	return buf.String()
}

func (pos Position) Format(f fmt.State, c rune) {
	switch {
	case pos.Filename != "" && pos.Line > 0:
		fmt.Fprintf(f, "%s:%d:%d", pos.Filename, pos.Line, pos.Column)
	case pos.Line > 0:
		fmt.Fprintf(f, "%d:%d", pos.Line, pos.Column)
	case pos.Filename != "":
		fmt.Fprint(f, pos.Filename)
	default:
		fmt.Fprint(f, "-")
	}
}

func parseExpr(s *ast.Scope, expr string) (ast.Expr, error) {
	n, err := parser.ParseExpr(types.FileSet, "<arg>", expr, s, types.DefaultImportPathToName)
	if err != nil {
		return nil, fmt.Errorf("cannot parse expression: %v", err)
	}
	switch n := n.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		return n, nil
	}
	return nil, fmt.Errorf("no identifier found in expression")
}

type FVisitor func(n ast.Node) bool

func (f FVisitor) Visit(n ast.Node) ast.Visitor {
	if f(n) {
		return f
	}
	return nil
}

var errNoPkgFiles = errors.New("no more package files found")

// parseLocalPackage reads and parses all go files from the
// current directory that implement the same package name
// the principal source file, except the original source file
// itself, which will already have been parsed.
//
func parseLocalPackage(filename string, src *ast.File, pkgScope *ast.Scope, pathToName parser.ImportPathToName) (*ast.Package, error) {
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
		src, err := parser.ParseFile(types.FileSet, file, nil, 0, pkg.Scope, types.DefaultImportPathToName)
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
	prog, _ := parser.ParseFile(types.FileSet, filename, nil, parser.PackageClauseOnly, nil, types.DefaultImportPathToName)
	if prog != nil {
		return prog.Name.Name
	}
	return ""
}

func hasSuffix(s, suff string) bool {
	return len(s) >= len(suff) && s[len(s)-len(suff):] == suff
}
