package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"go/token"
	"code.google.com/p/go.tools/go/loader"
	"code.google.com/p/go.tools/go/ssa"
	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/oracle"
	"github.com/davecgh/go-spew/spew"
)

var spewConf = spew.ConfigState{
	Indent:         "\t",
	DisableMethods: true,
	MaxDepth:       5,
}

var errorType = types.Universe.Lookup("error").Type()

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: errorpaths <args>\n")
		fmt.Fprint(os.Stderr, loader.FromArgsUsage)
	}
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
	}
	conf := loader.Config{
		SourceImports: true,
	}
	_, err := conf.FromArgs(args)
	if err != nil {
		log.Fatalf("cannot initialise loader: %v", err)
	}
	lprog, err := conf.Load()
	if err != nil {
		log.Fatalf("cannot load program: %v", err)
	}
	or, err := oracle.New(lprog, nil, false)
	if err != nil {
		log.Fatalf("cannot make oracle: %v", err)
	}
	ssaProg := ssa.Create(lprog, ssa.SanityCheckFunctions)
	ctxt := &context{
		lprog:   lprog,
		ssaProg: ssaProg,
		oracle:  or,
	}

	var foundPkg *types.Package
	log.Printf("searching %d packages", len(lprog.AllPackages))
	for pkg, _ := range lprog.AllPackages {
		log.Printf("looking at package %s", pkg.Path())
		if obj := pkg.Scope().Lookup("Test"); obj != nil {
			if _, ok := obj.Type().(*types.Signature); ok {
				foundPkg = pkg
			}
		}
	}
	if foundPkg == nil {
		log.Fatalf("couldn't find a package containing Test")
	}
	log.Printf("found package %s", foundPkg.Name())
	mainPkg := ssaProg.Package(foundPkg)
	mainPkg.Build()
	f := mainPkg.Func("Test")
	fmt.Printf("%+v\n", ctxt.errorPaths(f))
}

func (ctxt *context) errorPaths(f *ssa.Function) errorInfo {
	results := f.Signature.Results()
	if n := results.Len(); n == 0 || !types.IsIdentical(results.At(n-1).Type(), errorType) {
		return errorInfo{}
	}
	var info errorInfo
	seen := make(map[ssa.Value]bool)
	for _, b := range f.Blocks {
		if ret, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return); ok {
			info = info.add(ctxt.getErrorInfo(ret.Results[len(ret.Results)-1], 0, seen))
		}
	}
	return info
}

type context struct {
	ssaProg *ssa.Program
	lprog   *loader.Program
	oracle  *oracle.Oracle
}

func operands(inst ssa.Instruction) []ssa.Value {
	ops := inst.Operands(nil)
	vs := make([]ssa.Value, len(ops))
	for i, op := range ops {
		vs[i] = *op
	}
	return vs
}

type errorInfo struct {
	nonNil  int
	unknown int
}

func (a errorInfo) add(b errorInfo) errorInfo {
	a.nonNil += b.nonNil
	a.unknown += b.unknown
	return a
}

func (ctxt *context) getErrorInfo(v ssa.Value, member int, seen map[ssa.Value]bool) errorInfo {
	if seen[v] {
		return errorInfo{}
	}
	seen[v] = true
	defer delete(seen, v)
	switch v := v.(type) {
	case *ssa.Call:
		// TODO analyse call
		return errorInfo{unknown: 1}
	case *ssa.ChangeInterface:
		return ctxt.getErrorInfo(v.X, 0, seen)
	case *ssa.Extract:
		return ctxt.getErrorInfo(v.Tuple, v.Index, seen)
	case *ssa.Field:
		return errorInfo{unknown: 1}
	case *ssa.Index:
		return errorInfo{unknown: 1}
	case *ssa.Lookup:
		return errorInfo{unknown: 1}
	case *ssa.MakeInterface:
		if c, isNil := v.X.(*ssa.Const); isNil {
			// The only way of initialising an error from a constant is from nil.
			if c.Value != nil {
				panic("non-nil constant initializing error!")
			}
			return errorInfo{}
		}
		// TODO look at the value of val.X for component errors.
		return errorInfo{nonNil: 1}
	case *ssa.Next:
		return errorInfo{unknown: 1}
	case *ssa.Parameter:
		return errorInfo{unknown: 1}
	case *ssa.Phi:
		var info errorInfo
		for _, edge := range v.Edges {
			info = info.add(ctxt.getErrorInfo(edge, member, seen))
		}
		return info
	case *ssa.Select:
		return errorInfo{unknown: 1}
	case *ssa.TypeAssert:
		if v.CommaOk {
			return errorInfo{unknown: 1}
		}
		return ctxt.getErrorInfo(v.X, 0, seen)
	case *ssa.UnOp:
		if v.Op == token.ARROW {
			return errorInfo{unknown: 1}
		}
		panic(fmt.Errorf("unexpected unary operator %s", v))
	}
	panic(fmt.Errorf("unexpected value found for error: %T; %v", v, v))
}
