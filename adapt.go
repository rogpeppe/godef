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
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	rpast "github.com/rogpeppe/godef/go/ast"
	rpprinter "github.com/rogpeppe/godef/go/printer"
	rptypes "github.com/rogpeppe/godef/go/types"
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

func (b *triBool) Get() interface{} {
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

func detectModuleMode(cfg *packages.Config) bool {
	// first see if the config forces module mode
	for _, e := range cfg.Env {
		switch e {
		case "GO111MODULE=off":
			return false
		case "GO111MODULE=on":
			return true
		}
	}
	// do a fast test for go.mod in the working directory
	if _, err := os.Stat(filepath.Join(cfg.Dir, "go.mod")); !os.IsNotExist(err) {
		return true
	}
	// fall back to invoking the go tool to see if it will pick module mode
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Env = cfg.Env
	cmd.Dir = cfg.Dir
	out, err := cmd.Output()
	if err == nil {
		return len(strings.TrimSpace(string(out))) > 0
	}
	// default to non module mode
	return false
}

func adaptGodef(cfg *packages.Config, filename string, src []byte, searchpos int) (*Object, error) {
	usePackages := false
	switch forcePackages {
	case unset:
		usePackages = detectModuleMode(cfg)
	case on:
		usePackages = true
	case off:
		usePackages = false
	}
	if usePackages {
		fset, obj, err := godefPackages(cfg, filename, src, searchpos)
		if err != nil {
			return nil, err
		}
		return adaptGoObject(fset, obj)
	}
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
		if typ.Node != nil {
			result.Value = typ.Node.(*rpast.ImportSpec).Path.Value
		} else {
			result.Kind = PathKind
			result.Value = obj.Data.(string)
		}
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
	case gotypes.Type:
		buf := &bytes.Buffer{}
		gotypes.WriteType(buf, n, func(p *gotypes.Package) string { return "" })
		buf.WriteTo(f)
	default:
		fmt.Fprint(f, n)
	}
}
