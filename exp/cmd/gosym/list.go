package main

import (
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/sym"
	"code.google.com/p/rog-go/exp/go/types"
	"flag"
	"fmt"
	"log"
	"strings"
	"unicode"
)

type listCmd struct {
	all       bool
	verbose   bool
	printType bool
	kinds     string
	ctxt      *context
}

var listAbout = `
gosym list [flags] [pkg...]

The list command prints a line for each identifier
used in the named packages. Each line printed has at least 6 space-separated fields
in the following format:
	file-position referenced-file-position package referenced-package name type-kind
This format is known as "long" format.
If no packages are named, "." is used.

The file-position field holds the location of the identifier.
The referenced-file-position field holds the location of the
definition of the identifier.
The package field holds the path of the package containing the identifier.
The referenced-package field holds the path of the package
where the identifier is defined.
The name field holds the name of the identifier (in X.Y format if
it is defined as a member of another type X).
The type-kind field holds the type class of identifier (const,
type, var or func), and ends with a "+" sign if this line
marks the definition of the identifier.
`[1:]

func init() {
	c := &listCmd{}
	fset := flag.NewFlagSet("gosym list", flag.ExitOnError)
	fset.StringVar(&c.kinds, "k", allKinds(), "kinds of symbol types to include")
	fset.BoolVar(&c.verbose, "v", false, "print warnings about undefined symbols")
	fset.BoolVar(&c.printType, "t", false, "print symbol type")
	fset.BoolVar(&c.all, "a", false, "print internal symbols too")
	register("list", c, fset, listAbout)
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
	if info.Universe {
		return true
	}
	if !c.all && !isExported(info.Ident.Name) {
		return true
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
		//		c.ctxt.print("exprtype %s\n", pretty(e.X))
		name = e.Sel.Name
		switch xn := depointer(xt.Node).(type) {
		case nil:
			if c.verbose {
				log.Printf("%v: no type for %s", c.ctxt.position(e.Pos()), pretty(e.X))
			}
			return true
		case *ast.Ident:
			name = xn.Name + "." + name
		case *ast.ImportSpec:
			// don't qualify with package identifier
		default:
			// literal struct or interface expression.
			name = "_." + name
		}
	}
	line := &symLine{
		long:     true,
		pos:      eposition,
		referPos: c.ctxt.position(info.ReferPos),
		exprPkg:  exprPkg,
		referPkg: referPkg,
		local:    info.Local && info.ReferPos == info.Pos,
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

var objKinds = map[string]ast.ObjKind{
	"const": ast.Con,
	"type":  ast.Typ,
	"var":   ast.Var,
	"func":  ast.Fun,
}

func allKinds() string {
	var ks []string
	for k := range objKinds {
		ks = append(ks, k)
	}
	return strings.Join(ks, ",")
}
