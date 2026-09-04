package main

// The contents of this file are designed to adapt between the two implementations
// of godef, and should be removed when we fully switch to the go/pacakges
// implementation for all cases

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	gotoken "go/token"
	gotypes "go/types"

	"golang.org/x/tools/go/packages"
)

var forcePackages triBool

func init() {
	flag.Var(&forcePackages, "new-implementation", "force godef to use the new go/packages implentation")
}

// triBool is used as a unset, on or off valued flag
type triBool int

const (
	// unset means the triBool does not yet have a value
	unset = triBool(iota)
	// on means the triBool has been set to true
	on
	// off means the triBool has been set to false
	off
)

func (b *triBool) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if v {
		*b = on
	} else {
		*b = off
	}
	return err
}

func (b *triBool) Get() any {
	return *b
}

func (b *triBool) String() string {
	switch *b {
	case unset:
		return "default"
	case on:
		return "true"
	case off:
		return "false"
	default:
		return "invalid"
	}
}

func (b *triBool) IsBoolFlag() bool {
	return true
}

func adaptGodef(cfg *packages.Config, filename string, src []byte, searchpos int) (*Object, error) {
	fset, obj, err := godefPackages(cfg, filename, src, searchpos)
	if err != nil {
		return nil, err
	}
	return adaptGoObject(fset, obj)
}

func adaptGoObject(fset *gotoken.FileSet, obj gotypes.Object) (*Object, error) {
	result := &Object{
		Name:     obj.Name(),
		Position: objToPos(fset, obj),
		Type:     obj.Type(),
	}
	switch obj := obj.(type) {
	case *gotypes.Func:
		result.Kind = FuncKind
	case *gotypes.Var:
		result.Kind = VarKind
	case *gotypes.PkgName:
		result.Kind = ImportKind
		result.Type = nil
		if obj.Pkg() != nil {
			result.Value = strconv.Quote(obj.Imported().Path())
		} else {
			result.Value = obj.Imported().Path()
			result.Kind = PathKind
		}
	case *gotypes.Const:
		result.Kind = ConstKind
		result.Value = obj.Val()
	case *gotypes.Label:
		result.Kind = LabelKind
		result.Type = nil
	case *gotypes.TypeName:
		result.Kind = TypeKind
		result.Type = obj.Type().Underlying()
	default:
		result.Kind = BadKind
	}

	return result, nil
}

func objToPos(fSet *gotoken.FileSet, obj gotypes.Object) Position {
	p := obj.Pos()
	f := fSet.File(p)
	goPos := f.Position(p)
	pos := Position{
		Filename: cleanFilename(goPos.Filename),
		Line:     goPos.Line,
		Column:   goPos.Column,
	}
	if pos.Column != 1 {
		return pos
	}
	// currently exportdata does not store the column
	// until it does, we have a hacky fix to attempt to find the name within
	// the line and patch the column to match
	named, ok := obj.(interface{ Name() string })
	if !ok {
		return pos
	}
	in, err := os.Open(f.Name())
	if err != nil {
		return pos
	}
	for l, scanner := 1, bufio.NewScanner(in); scanner.Scan(); l++ {
		if l < pos.Line {
			continue
		}
		col := bytes.Index([]byte(scanner.Text()), []byte(named.Name()))
		if col >= 0 {
			pos.Column = col + 1
		}
		break
	}
	return pos
}

// cleanFilename normalizes any file names that come out of the fileset.
func cleanFilename(path string) string {
	const prefix = "$GOROOT"
	if len(path) < len(prefix) || !strings.EqualFold(prefix, path[:len(prefix)]) {
		return path
	}
	//TODO: we need a better way to get the GOROOT that uses the packages api
	return runtime.GOROOT() + path[len(prefix):]
}

type pretty struct {
	n any
}

func (p pretty) Format(f fmt.State, c rune) {
	switch n := p.n.(type) {
	case gotypes.Type:
		buf := &bytes.Buffer{}
		gotypes.WriteType(buf, n, func(p *gotypes.Package) string { return "" })
		buf.WriteTo(f)
	default:
		fmt.Fprint(f, n)
	}
}
