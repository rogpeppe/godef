package main

// The contents of this file are designed to adapt between the two implementations
// of godef, and should be removed when we fully switch to the go/pacakges
// implementation for all cases

import (
	"fmt"
	"golang.org/x/tools/go/packages"
	"sort"

	rpast "github.com/rogpeppe/godef/go/ast"
	rpprinter "github.com/rogpeppe/godef/go/printer"
	rptypes "github.com/rogpeppe/godef/go/types"
)

func adaptGodef(cfg *packages.Config, filename string, src []byte, searchpos int) (*Object, error) {
	obj, typ, err := godef(filename, src, searchpos)
	if err != nil {
		return nil, err
	}
	return adaptRPObject(obj, typ)
}

func adaptRPObject(obj *rpast.Object, typ rptypes.Type) (*Object, error) {
	pos := rptypes.FileSet.Position(rptypes.DeclPos(obj))
	result := &Object{
		Name: obj.Name,
		Pkg:  typ.Pkg,
		Position: Position{
			Filename: pos.Filename,
			Line:     pos.Line,
			Column:   pos.Column,
		},
		Type: typ,
	}
	switch obj.Kind {
	case rpast.Bad:
		result.Kind = BadKind
	case rpast.Fun:
		result.Kind = FuncKind
	case rpast.Var:
		result.Kind = VarKind
	case rpast.Pkg:
		result.Kind = ImportKind
		result.Type = nil
		result.Value = typ.Node.(*rpast.ImportSpec).Path.Value
	case rpast.Con:
		result.Kind = ConstKind
		if decl, ok := obj.Decl.(*rpast.ValueSpec); ok {
			result.Value = decl.Values[0]
		}
	case rpast.Lbl:
		result.Kind = LabelKind
		result.Type = nil
	case rpast.Typ:
		result.Kind = TypeKind
		result.Type = typ.Underlying(false)
	}
	for child := range typ.Iter() {
		m, err := adaptRPObject(child, rptypes.Type{})
		if err != nil {
			return nil, err
		}
		result.Members = append(result.Members, m)
	}
	sort.Sort(orderedObjects(result.Members))
	return result, nil
}

type pretty struct {
	n interface{}
}

func (p pretty) Format(f fmt.State, c rune) {
	switch n := p.n.(type) {
	case *rpast.BasicLit:
		rpprinter.Fprint(f, rptypes.FileSet, n)
	case rptypes.Type:
		// TODO print path package when appropriate.
		// Current issues with using p.n.Pkg:
		//	- we should actually print the local package identifier
		//	rather than the package path when possible.
		//	- p.n.Pkg is non-empty even when
		//	the type is not relative to the package.
		rpprinter.Fprint(f, rptypes.FileSet, n.Node)
	default:
		fmt.Fprint(f, n)
	}
}
