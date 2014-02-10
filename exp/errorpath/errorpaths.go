package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"

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
		fmt.Fprintf(os.Stderr, "Usage: errorpaths scope pkg-pattern\n")
		fmt.Fprint(os.Stderr, loader.FromArgsUsage)
	}
	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		flag.Usage()
	}
	conf := loader.Config{
		SourceImports: true,
	}
	_, err := conf.FromArgs(args[0:1])
	if err != nil {
		log.Fatalf("cannot initialise loader: %v", err)
	}
	pkgPat, err := regexp.Compile("^" + args[1] + "$")
	if err != nil {
		log.Fatalf("cann compile regexp %q: %s", args[1], err)
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
		infos:   make(map[*ssa.Function]*errorInfo),
		locs: make(map[*ssa.Function] errorLocations),
	}

	var foundPkgs []*types.Package
	log.Printf("searching %d packages", len(lprog.AllPackages))

	for pkg, _ := range lprog.AllPackages {
		if pkgPat.MatchString(pkg.Path()) {
			foundPkgs = append(foundPkgs, pkg)
			break
		}
	}
	if len(foundPkgs) == 0 {
		log.Fatalf("failed to find any matching packages")
	}
	for _, pkg := range foundPkgs {
		log.Printf("package %s", pkg.Name())
		ssaPkg := ssaProg.Package(pkg)
		ssaPkg.Build()
		for name, m := range ssaPkg.Members {
			log.Printf("name %s", name)
			if f, ok := m.(*ssa.Function); ok && returnsError(f) {
				fmt.Printf("%s\n", f)
				locs := ctxt.errorLocations(f)
				ctxt.dumpErrorLocs(locs, os.Stdout, "\t")
			}
		}
	}
}

type errorLocations struct {
	nonNil  [][]token.Pos
	unknown [][]token.Pos
}

func (ctxt *context) errorLocations(f *ssa.Function) (result errorLocations) {
	log.Printf("errorLocations %s {", f)
	defer func() {
		log.Printf("} -> %v", result)
	}()
	if locs, ok := ctxt.locs[f]; ok {
		log.Printf("errorLocations already calculated for %s", f)
		return locs
	}
	ctxt.locs[f] = errorLocations{}		// Prevent runaway recursion.
	info := ctxt.infos[f]
	if info == nil {
		info = ctxt.errorPaths(f)
		ctxt.infos[f] = info
	} else {
		log.Printf("already have errorPaths for %s", f)
	}
	var locs errorLocations
	for _, t := range info.nonNil {
		if !t.pos.IsValid() {
			log.Printf("%s does not have valid position", t.val)
			continue
		}
		locs.nonNil = append(locs.nonNil, []token.Pos{t.pos})
	}
	for _, t := range info.unknown {
		locs.unknown = append(locs.unknown, []token.Pos{t.pos})
	}
	for _, call := range info.nested {
		fs, err := ctxt.callees(call)
		if err != nil {
			log.Printf("cannot get callees for %v: %v", call, err)
			continue
		}
		for _, f := range fs {
			// TODO guard against infinite recursion
			callLocs := ctxt.errorLocations(f)
			locs.nonNil = append(locs.nonNil, insertPos(call.Pos(), callLocs.nonNil)...)
			locs.unknown = append(locs.unknown, insertPos(call.Pos(), callLocs.unknown)...)
		}
	}
	// Remember result for later.
	ctxt.locs[f] = locs
	return locs
}

// valPos tries to find a position for the given value
// even if the value doesn't have one.
func valPos(val ssa.Value) token.Pos {
	if pos := val.Pos(); pos.IsValid() {
		return pos
	}
	instr, ok := val.(ssa.Instruction)
	if !ok {
		return token.NoPos
	}
	for _, op := range instr.Operands(nil) {
		if pos := (*op).Pos(); pos.IsValid() {
			return pos
		}
	}
	return token.NoPos
}

func insertPos(pos token.Pos, traces [][]token.Pos) [][]token.Pos {
	r := make([][]token.Pos, len(traces))
	for i, trace := range traces {
		r[i] = make([]token.Pos, len(trace)+1)
		r[i][0] = pos
		copy(r[i][1:], trace)
	}
	return r
}

func (ctxt *context) dumpErrorLocs(locs errorLocations, w io.Writer, indent string) {
	print := func(s string, a ...interface{}) {
		fmt.Fprintf(w, "%s%s\n", indent, fmt.Sprintf(s, a...))
	}
	dumpTraces := func(traces [][]token.Pos) {
		for _, trace := range traces {
			for _, pos := range trace {
				print("\t%s", ctxt.lprog.Fset.Position(pos))
				// TODO dump source.
			}
			print("")
		}
	}
	print("non-nil")
	dumpTraces(locs.nonNil)
	print("unknown")
	dumpTraces(locs.unknown)
}

func returnsError(f *ssa.Function) bool {
	results := f.Signature.Results()
	n := results.Len()
	return n > 0 && types.IsIdentical(results.At(n-1).Type(), errorType)
}

func (ctxt *context) errorPaths(f *ssa.Function) (result *errorInfo) {
	log.Printf("errorPaths %s (synthetic %q) {", f, f.Synthetic)
	defer func() {
		log.Printf("} -> %+v", result)
	}()
	if !returnsError(f) {
		return &errorInfo{}
	}
	var info errorInfo
	seen := make(map[ssa.Value]bool)
	for _, b := range f.Blocks {
		if ret, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return); ok {
			if !ret.Pos().IsValid() {
				panicf("return operation has invalid position")
			}
//			log.Printf("return operands: %q", operands(ret))
			info.add(ctxt.getErrorInfo(ret.Results[len(ret.Results)-1], 0, ret.Pos(), seen))
		}
	}
	return &info
}

func panicf(s string, a ...interface{}) {
	m := fmt.Sprintf(s, a...)
	fmt.Printf("panic: %s", m)
	panic(errors.New(m))
}

type context struct {
	ssaProg *ssa.Program
	lprog   *loader.Program
	oracle  *oracle.Oracle
	infos   map[*ssa.Function]*errorInfo
	locs map[*ssa.Function] errorLocations
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
	nonNil  []errorTermination
	unknown []errorTermination
	nested  []*ssa.Call
}

func (a *errorInfo) add(b *errorInfo) {
	a.nonNil = append(a.nonNil, b.nonNil...)
	a.unknown = append(a.unknown, b.unknown...)
	a.nested = append(a.nested, b.nested...)
}

type errorTermination struct {
	val ssa.Value
	pos token.Pos
}

func (ctxt *context) getErrorInfo(v ssa.Value, member int, enclosingPos token.Pos, seen map[ssa.Value]bool) (result *errorInfo) {
	if !enclosingPos.IsValid() {
		panicf("getErrorInfo with invalid pos; %T %s", v, v)
	}
//	log.Printf("getErrorInfo[%d] %T %v {", member, v, v)
//	defer func() {
//		log.Printf("} -> %+v", result)
//	}()

	if seen[v] {
		return &errorInfo{}
	}
	seen[v] = true
	defer delete(seen, v)
	if pos := v.Pos(); pos.IsValid() {
		enclosingPos = pos
	}
	terminate := func() []errorTermination {
		return []errorTermination{{
			val: v,
			pos: enclosingPos,
		}}
	}
	switch v := v.(type) {
	case *ssa.Call:
		if member > 0 && member != v.Type().(*types.Tuple).Len()-1 {
			log.Printf("error from non-final member of function")
			return &errorInfo{unknown: terminate()}
		}
		return &errorInfo{nested: []*ssa.Call{v}}
	case *ssa.ChangeInterface:
		return ctxt.getErrorInfo(v.X, 0, enclosingPos, seen)
	case *ssa.Extract:
		return ctxt.getErrorInfo(v.Tuple, v.Index, enclosingPos, seen)
	case *ssa.Field:
		return &errorInfo{unknown: terminate()}
	case *ssa.Index:
		return &errorInfo{unknown: terminate()}
	case *ssa.Lookup:
		return &errorInfo{unknown: terminate()}
	case *ssa.Const:
		if v.Value != nil {
			panicf("non-nil constant cannot make error, surely?")
		}
		return &errorInfo{}
	case *ssa.MakeInterface:
		// TODO look into components of v.X
		return &errorInfo{nonNil: terminate()}
	case *ssa.Next:
		return &errorInfo{unknown: terminate()}
	case *ssa.Parameter:
		return &errorInfo{unknown: terminate()}
	case *ssa.Phi:
		var info errorInfo
		for _, edge := range v.Edges {
			info.add(ctxt.getErrorInfo(edge, member, enclosingPos, seen))
		}
		return &info
	case *ssa.Select:
		return &errorInfo{unknown: terminate()}
	case *ssa.TypeAssert:
		if v.CommaOk {
			return &errorInfo{unknown: terminate()}
		}
		return ctxt.getErrorInfo(v.X, 0, enclosingPos, seen)
	case *ssa.UnOp:
		switch v.Op {
		case token.ARROW:
			return &errorInfo{unknown: terminate()}
		case token.MUL:
			if _, isGlobal := v.X.(*ssa.Global); isGlobal {
				// Assume that if we're returning a global variable, it's a
				// global non-nil error, such as os.ErrInvalid.
				return &errorInfo{nonNil: terminate()}
			}
			return &errorInfo{unknown: terminate()}
		default:
			panicf("unexpected unary operator %s at %s", v, ctxt.lprog.Fset.Position(enclosingPos))
		}
	}
	panicf("unexpected value found for error: %T; %v", v, v)
	panic("not reached")
}
