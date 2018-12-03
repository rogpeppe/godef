package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

func godefPackages(cfg *packages.Config, filename string, src []byte, searchpos int) (*token.FileSet, types.Object, error) {
	parser, result := parseFile(filename, searchpos)
	// Load, parse, and type-check the packages named on the command line.
	if src != nil {
		cfg.Overlay = map[string][]byte{
			filename: src,
		}
	}
	cfg.Mode = packages.LoadSyntax
	cfg.ParseFile = parser
	lpkgs, err := packages.Load(cfg, "file="+filename)
	if err != nil {
		return nil, nil, err
	}
	if len(lpkgs) < 1 {
		return nil, nil, fmt.Errorf("There must be at least one package that contains the file")
	}
	// get the node
	var m match
	select {
	case m = <-result:
	default:
		return nil, nil, fmt.Errorf("no file found at search pos %d", searchpos)
	}
	if m.ident == nil {
		return nil, nil, fmt.Errorf("Offset %d was not a valid identifier", searchpos)
	}
	obj := lpkgs[0].TypesInfo.ObjectOf(m.ident)
	if obj == nil {
		return nil, nil, fmt.Errorf("no object")
	}
	if m.wasEmbeddedField {
		// the original position was on the embedded field declaration
		// so we try to dig out the type and jump to that instead
		if v, ok := obj.(*types.Var); ok {
			if n, ok := v.Type().(*types.Named); ok {
				obj = n.Obj()
			}
		}
	}
	return lpkgs[0].Fset, obj, nil
}

// match returns the ident plus any extra information needed
type match struct {
	ident            *ast.Ident
	wasEmbeddedField bool
}

// parseFile returns a function that can be used as a Parser in packages.Config.
// It replaces the contents of a file that matches filename with the src.
// It also drops all function bodies that do not contain the searchpos.
// It also modifies the filename to be the canonical form that will appear in the fileset.
func parseFile(filename string, searchpos int) (func(*token.FileSet, string, []byte) (*ast.File, error), chan match) {
	result := make(chan match, 1)
	isInputFile := newFileCompare(filename)
	return func(fset *token.FileSet, fname string, filedata []byte) (*ast.File, error) {
		isInput := isInputFile(fname)
		file, err := parser.ParseFile(fset, fname, filedata, 0)
		if file == nil {
			return nil, err
		}
		pos := token.Pos(-1)
		if isInput {
			tfile := fset.File(file.Pos())
			if tfile == nil {
				return file, fmt.Errorf("cursor %d is beyond end of file %s (%d)", searchpos, fname, file.End()-file.Pos())
			}
			if searchpos > tfile.Size() {
				return file, fmt.Errorf("cursor %d is beyond end of file %s (%d)", searchpos, fname, tfile.Size())
			}
			pos = tfile.Pos(searchpos)
			m, err := findMatch(file, pos)
			if err != nil {
				return nil, err
			}
			result <- m
		}
		trimAST(file, pos)
		return file, err
	}, result
}

func newFileCompare(filename string) func(string) bool {
	fstat, fstatErr := os.Stat(filename)
	return func(compare string) bool {
		if filename == compare {
			return true
		}
		if fstatErr != nil {
			return false
		}
		if s, err := os.Stat(compare); err == nil {
			return os.SameFile(fstat, s)
		}
		return false
	}
}

func findMatch(f *ast.File, pos token.Pos) (match, error) {
	m, err := checkMatch(f, pos)
	if err != nil {
		return match{}, err
	}
	if m.ident != nil {
		return m, nil
	}
	// If the position is not an identifier but immediately follows
	// an identifier or selector period (as is common when
	// requesting a completion), use the path to the preceding node.
	return checkMatch(f, pos-1)
}

// checkMatch checks a single position for a potential identifier.
func checkMatch(f *ast.File, pos token.Pos) (match, error) {
	path, _ := astutil.PathEnclosingInterval(f, pos, pos)
	result := match{}
	if path == nil {
		return result, fmt.Errorf("can't find node enclosing position")
	}
	switch node := path[0].(type) {
	case *ast.Ident:
		result.ident = node
	case *ast.SelectorExpr:
		result.ident = node.Sel
	}
	if result.ident != nil {
		for _, n := range path[1:] {
			if field, ok := n.(*ast.Field); ok {
				result.wasEmbeddedField = len(field.Names) == 0
			}
		}
	}
	return result, nil
}

func trimAST(file *ast.File, pos token.Pos) {
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if pos < n.Pos() || pos >= n.End() {
			switch n := n.(type) {
			case *ast.FuncDecl:
				n.Body = nil
			case *ast.BlockStmt:
				n.List = nil
			case *ast.CaseClause:
				n.Body = nil
			case *ast.CommClause:
				n.Body = nil
			case *ast.CompositeLit:
				// Leave elts in place for [...]T
				// array literals, because they can
				// affect the expression's type.
				if !isEllipsisArray(n.Type) {
					n.Elts = nil
				}
			}
		}
		return true
	})
}

func isEllipsisArray(n ast.Expr) bool {
	at, ok := n.(*ast.ArrayType)
	if !ok {
		return false
	}
	_, ok = at.Len.(*ast.Ellipsis)
	return ok
}
