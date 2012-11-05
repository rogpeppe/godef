package main

import (
	"bytes"
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/parser"
	"code.google.com/p/rog-go/exp/go/printer"
	"code.google.com/p/rog-go/exp/go/token"
	"code.google.com/p/rog-go/exp/go/types"
	"strconv"
)

type symInfo struct {
	pos      token.Pos   // position of symbol.
	expr     ast.Expr    // expression for symbol (*ast.Ident or *ast.SelectorExpr)
	ident    *ast.Ident  // identifier in parse tree (changing ident.Name changes the parse tree)
	exprType types.Type  // type of expression.
	referPos token.Pos   // position of referred-to symbol.
	referObj *ast.Object // object referred to. 
	local    bool        // whether referred-to object is function-local.
	universe bool        // whether referred-to object is in universe.
}

type vcontext struct {
	importer types.Importer
	logf     func(pos token.Pos, f string, a ...interface{})
}

// visitSyms calls visitf for each identifier in the given file.
func (ctxt *vcontext) visitSyms(pkg *ast.File, visitf func(*symInfo) bool) {
	var visit astVisitor
	ok := true
	local := false // TODO set to true inside function body
	visit = func(n ast.Node) bool {
		if !ok {
			return false
		}
		switch n := n.(type) {
		case *ast.ImportSpec:
			// If the file imports a package to ".", abort
			// because we don't support that (yet).
			if n.Name != nil && n.Name.Name == "." {
				ctxt.logf(n.Pos(), "import to . not supported")
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
			ok = ctxt.visitExpr(n, local, visitf)
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
			ok = ctxt.visitExpr(n, local, visitf)
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

func (ctxt *vcontext) visitExpr(e ast.Expr, local bool, visitf func(*symInfo) bool) bool {
	var info symInfo
	info.expr = e
	switch e := e.(type) {
	case *ast.Ident:
		info.pos = e.Pos()
		info.ident = e
	case *ast.SelectorExpr:
		info.pos = e.Sel.Pos()
		info.ident = e.Sel
	}
	obj, t := types.ExprType(e, ctxt.importer)
	if obj == nil {
		ctxt.logf(e.Pos(), "no object for %s", pretty(e))
		return true
	}
	info.exprType = t
	info.referObj = obj
	if parser.Universe.Lookup(obj.Name) != obj {
		info.referPos = types.DeclPos(obj)
		if info.referPos == token.NoPos {
			ctxt.logf(e.Pos(), "no declaration for %s", pretty(e))
			return true
		}
	} else {
		info.universe = true
	}
	info.local = local
	return visitf(&info)
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

type astVisitor func(n ast.Node) bool

func (f astVisitor) Visit(n ast.Node) ast.Visitor {
	if f(n) {
		return f
	}
	return nil
}

var emptyFileSet = token.NewFileSet()

func pretty(n ast.Node) string {
	var b bytes.Buffer
	printer.Fprint(&b, emptyFileSet, n)
	return b.String()
}
