package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"io"
	"log"
	"os"
	"runtime"
	debugpkg "runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"strings"

	"golang.org/x/tools/go/packages"
)

var readStdin = flag.Bool("i", false, "read file from stdin")
var offset = flag.Int("o", -1, "file offset of identifier in stdin")
var fflag = flag.String("f", "", "Go source filename")
var acmeFlag = flag.Bool("acme", false, "use current acme window")
var jsonFlag = flag.Bool("json", false, "output location in JSON format (-t flag is ignored)")

var cpuprofile = flag.String("cpuprofile", "", "write CPU profile to this file")
var memprofile = flag.String("memprofile", "", "write memory profile to this file")
var traceFlag = flag.String("trace", "", "write trace log to this file")

var tflag = flag.Bool("t", false, "print type information")
var aflag = flag.Bool("a", false, "print public type and member information")
var Aflag = flag.Bool("A", false, "print all type and members information")

// Deprecated flags:
var (
	_ = flag.Bool("debug", false, "deprecated flag (ignored)")
)

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
	*tflag = *tflag || *aflag || *Aflag
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
		src, _ = io.ReadAll(os.Stdin)
	} else {
		// TODO if there's no filename, look in the current
		// directory and do something plausible.
		b, err := os.ReadFile(filename)
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
	Type     any
	Value    any
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

type FVisitor func(n ast.Node) bool

func (f FVisitor) Visit(n ast.Node) ast.Visitor {
	if f(n) {
		return f
	}
	return nil
}
