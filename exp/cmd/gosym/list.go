package main
import (
	"code.google.com/p/rog-go/exp/go/types"
	"code.google.com/p/rog-go/exp/go/sym"
	"code.google.com/p/rog-go/exp/go/ast"
	"flag"
	"log"
	"fmt"
	"strings"
	"unicode"
)

type listCmd struct {
	all bool
	verbose bool
	printType bool
	kinds string
	ctxt *context
}

func init() {
	c := &listCmd{}
	fset := flag.NewFlagSet("gosym list", flag.ExitOnError)
	fset.StringVar(&c.kinds, "k", allKinds(), "kinds of symbol types to include")
	fset.BoolVar(&c.verbose, "v", false, "print warnings about undefined symbols")
	fset.BoolVar(&c.printType, "t", false, "print symbol type")
	fset.BoolVar(&c.all, "a", false, "print internal and universe symbols too")
	register("list", c, fset)
}

func (c *listCmd) run(ctxt *context, args []string) error {
	c.ctxt = ctxt
	if c.kinds == "" {
		return fmt.Errorf("no type kinds specified")
	}
	mask, err := parseKindMask(c.kinds)
	if err != nil {
		return err
	}
	pkgs := args
	if len(pkgs) == 0 {
		pkgs = []string{"."}
	}
	visitor := func(info *sym.Info) bool {
		return c.visit(info, mask)
	}
	for _, path := range pkgs {
		if pkg := ctxt.Import(path); pkg != nil {
			for _, f := range pkg.Files {
				ctxt.IterateSyms(f, visitor)
			}
		}
	}
	return nil
}

func isExported(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}

func (c *listCmd) visit(info *sym.Info, kindMask uint) bool {
	if (1<<uint(info.ReferObj.Kind))&kindMask == 0 {
		return true
	}
	if !c.all {
		if info.Universe || !isExported(info.Ident.Name) {
			return true
		}
	}
	eposition := c.ctxt.position(info.Pos)
	exprPkg := c.ctxt.positionToImportPath(eposition)
	var referPkg string
	if info.Universe {
		referPkg = "universe"
	} else {
		referPkg = c.ctxt.positionToImportPath(c.ctxt.position(info.ReferPos))
	}
	name := info.Ident.Name
	if e, ok := info.Expr.(*ast.SelectorExpr); ok {
		_, xt := types.ExprType(e.X, func(path string) *ast.Package {
			return c.ctxt.Import(path)
		})
		if xt.Node == nil {
			if c.verbose {
				log.Printf("%v: no type for %s", c.ctxt.position(e.Pos()), pretty(e.X))
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
	if c.printType {
		line.exprType = pretty(info.ExprType.Node)
	}
	c.ctxt.printf("%s\n", line)
	return true
}

func depointer(x ast.Node) ast.Node {
	if x, ok := x.(*ast.StarExpr); ok {
		return x.X
	}
	return x
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
