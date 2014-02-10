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
		switch n := n.(type) {
		case *ast.CallExpr:
			switch {
			case isPkgDot(n.Fun, "fmt", "Errorf"):
				if len(n.Args) == 0 {
					log.Printf("warning: Errorf with no args")
					break
				}
				lit, ok := n.Args[0].(*ast.BasicLit)
				if !ok {
					log.Printf("warning: Errorf with non-constant first arg")
					break
				}
				if lit.Kind != token.STRING {
					log.Printf("warning: Errorf with non-string literal first arg")
					break
				}
				format, err := strconv.Unquote(lit.Value)
				if err != nil {
					log.Printf("warning: Errorf with invalid quoted string literal: %v", err)
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
			}
		case *ast.IfStmt:
			if ok := fixIfErrNotEqualNil(n, errgoIdent); ok {
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
	// if err != nil {
	//	return [..., ]err
	//  }
	// ->
	// if err != nil {
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
