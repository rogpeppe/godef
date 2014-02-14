package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"path"
	"strconv"
	"strings"
)

func init() {
	register(errgoFix)
}

var errgoFix = fix{
	"errgo",
	"2014-02-10",
	errgo,
	`Use errgo instead of fmt.Errorf, etc
`,
}

func importPathToIdentMap(f *ast.File) map[string]string {
	m := make(map[string]string)
	for _, imp := range f.Imports {
		ipath := importPath(imp)
		if imp.Name != nil {
			m[ipath] = imp.Name.Name
		} else {
			_, name := path.Split(ipath)
			m[ipath] = name
		}
	}
	return m
}

func errgo(f *ast.File) bool {
	if imports(f, "launchpad.net/errgo/errors") {
		return false
	}
	pathToIdent := importPathToIdentMap(f)
	gocheckIdent := pathToIdent["launchpad.net/gocheck"]

	// If we import from any */errors package path,
	// import as errgo to save name clashes.
	errgoIdent := "errors"
	for _, imp := range f.Imports {
		path := importPath(imp)
		if strings.HasSuffix(path, "/errors") {
			errgoIdent = "errgo"
		}
	}

	fixed := false
	walk(f, func(n interface{}) {
		warning := func(format string, arg ...interface{}) {
			pos := fset.Position(n.(ast.Node).Pos())
			log.Printf("warning: %s: %s", pos, fmt.Sprintf(format, arg...))
		}
		switch n := n.(type) {
		case *ast.CallExpr:
			switch {
			case isPkgDot(n.Fun, "fmt", "Errorf"):
				if len(n.Args) == 0 {
					warning("Errorf with no args")
					break
				}
				lit, ok := n.Args[0].(*ast.BasicLit)
				if !ok {
					warning("Errorf with non-constant first arg")
					break
				}
				if lit.Kind != token.STRING {
					warning("Errorf with non-string literal first arg")
					break
				}
				format, err := strconv.Unquote(lit.Value)
				if err != nil {
					warning("Errorf with invalid quoted string literal: %v", err)
					break
				}
				if !strings.HasSuffix(format, ": %v") || len(n.Args) < 2 || !isName(n.Args[len(n.Args)-1], "err") {
					// fmt.Errorf("foo %s", x) ->
					// errgo.Newf("foo %s", x)
					n.Fun = &ast.SelectorExpr{
						X:   ast.NewIdent(errgoIdent),
						Sel: ast.NewIdent("Newf"),
					}
					fixed = true
					break
				}
				// fmt.Errorf("format: %v", args..., err) ->
				// errgo.Wrapf(err, "format", args...)
				newArgs := []ast.Expr{
					n.Args[len(n.Args)-1],
					&ast.BasicLit{
						Kind:  token.STRING,
						Value: fmt.Sprintf("%q", strings.TrimSuffix(format, ": %v")),
					},
				}
				newArgs = append(newArgs, n.Args[1:len(n.Args)-1]...)
				n.Args = newArgs
				n.Fun = &ast.SelectorExpr{
					X:   ast.NewIdent(errgoIdent),
					Sel: ast.NewIdent("Wrapf"),
				}
				fixed = true
			case isPkgDot(n.Fun, "errgo", "Annotate"):
				n.Fun = &ast.SelectorExpr{
					X:   ast.NewIdent(errgoIdent),
					Sel: ast.NewIdent("WrapMsg"),
				}
				fixed = true
			case isPkgDot(n.Fun, "errgo", "Annotatef"):
				n.Fun = &ast.SelectorExpr{
					X:   ast.NewIdent(errgoIdent),
					Sel: ast.NewIdent("Wrapf"),
				}
				fixed = true
			case isPkgDot(n.Fun, "errgo", "New"):
				n.Fun = &ast.SelectorExpr{
					X:   ast.NewIdent(errgoIdent),
					Sel: ast.NewIdent("Newf"),
				}
				fixed = true
			case isPkgDot(n.Fun, pathToIdent["errors"], "New"):
				n.Fun = &ast.SelectorExpr{
					X:   ast.NewIdent(errgoIdent),
					Sel: ast.NewIdent("New"),
				}
				fixed = true
			case fixGocheck(n, errgoIdent, gocheckIdent):
				fixed = true
			}
		case *ast.IfStmt:
			if ok := fixIfErrNotEqualNil(n, errgoIdent); ok {
				fixed = true
				break
			}
			if ok := fixIfErrEqualSomething(n, errgoIdent); ok {
				fixed = true
				break
			}
		}
	})
	fixed = deleteImport(f, "github.com/errgo/errgo") || fixed
	// If there was already an "errors" import, then we can
	// rewrite it to use errgo
	if pathToIdent["errors"] != "" {
		// We've already imported the errors package;
		// change it to refer to errgo.
		for _, imp := range f.Imports {
			if importPath(imp) == "errors" {
				fixed = true
				imp.EndPos = imp.End()
				imp.Path.Value = strconv.Quote("launchpad.net/errgo/errors")
				if errgoIdent != "errors" {
					imp.Name = ast.NewIdent(errgoIdent)
				}
			}
		}
	} else {
		fixed = addImport(f, "launchpad.net/errgo/errors", errgoIdent, false) || fixed
	}
	return fixed
}

func fixIfErrNotEqualNil(n *ast.IfStmt, errgoIdent string) bool {
	// if stmt; err != nil {
	//	return [..., ]err
	//  }
	// ->
	// if stmt; err != nil {
	// 	return [..., ]errgo.Wrap(err)
	// }
	cond, ok := n.Cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if !isName(cond.X, "err") {
		return false
	}
	if !isName(cond.Y, "nil") {
		// comparison of errors against anything
		// other than nil - use errgo.Diagnosis.

	}
	if cond.Op != token.NEQ {
		return false
	}
	if len(n.Body.List) != 1 {
		return false
	}
	returnStmt, ok := n.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	if len(returnStmt.Results) == 0 {
		return false
	}
	lastResult := &returnStmt.Results[len(returnStmt.Results)-1]
	if !isName(*lastResult, "err") {
		return false
	}
	*lastResult = &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(errgoIdent),
			Sel: ast.NewIdent("Wrap"),
		},
		Args: []ast.Expr{ast.NewIdent("err")},
	}
	return true
}

func fixIfErrEqualSomething(n *ast.IfStmt, errgoIdent string) bool {
	// if stmt; err == something-but-not-nil
	// ->
	// if stmt; errgo.Diagnosis(err) == something-but-not-nil
	cond, ok := n.Cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if !isName(cond.X, "err") {
		return false
	}
	if cond.Op != token.EQL {
		return false
	}
	if isName(cond.Y, "nil") {
		return false
	}
	cond.X = &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(errgoIdent),
			Sel: ast.NewIdent("Diagnosis"),
		},
		Args: []ast.Expr{ast.NewIdent("err")},
	}
	return true
}


func fixGocheck(n *ast.CallExpr, errgoIdent, gocheckIdent string) bool {
	// gc.Check(err, gc.Equals, foo-not-nil)
	// ->
	// gc.Check(errgo.Diagnosis(err), gc.Equals, foo-not-nil)

	// gc.Check(err, gc.Not(gc.Equals), foo-not-nil)
	// ->
	// gc.Check(errgo.Diagnosis(err), gc.Not(gc.Equals), foo-not-nil)
	if gocheckIdent == "" {
		return false
	}
	sel, ok := n.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if !isName(sel.X, "c") {
		return false
	}
	if s := sel.Sel.String(); s != "Check" && s != "Assert" {
		return false
	}

	if len(n.Args) < 3 {
		return false
	}
	if !isName(n.Args[0], "err") {
		return false
	}
	if condCall, ok := n.Args[1].(*ast.CallExpr); ok {
		if !isPkgDot(condCall.Fun, gocheckIdent, "Not") {
			return false
		}
		if len(condCall.Args) != 1 {
			return false
		}
		if !isPkgDot(condCall.Args[0], gocheckIdent, "Equals") {
			return false
		}
	} else if !isPkgDot(n.Args[1], gocheckIdent, "Equals") {
		return false
	}
	if isName(n.Args[2], "nil") {
		return false
	}
	n.Args[0] = &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(errgoIdent),
			Sel: ast.NewIdent("Diagnosis"),
		},
		Args: []ast.Expr{ast.NewIdent("err")},
	}
	return true
}
