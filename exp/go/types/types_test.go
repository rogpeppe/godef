package types

import (
	"bytes"
	"exec"
	"io"
	"strings"
	"testing"
	"unicode"
	"go/token"
	"go/ast"
	"rog-go.googlecode.com/hg/exp/go/parser"
)

// TODO test cross-package symbols

// TestCompile checks that the test code actually compiles.
func TestCompile(t *testing.T) {
	code, _ := translateSymbols(testCode)
	c, err := exec.Run("/bin/sh", []string{"/bin/sh", "-c", "6g /dev/fd/0"}, nil, "", exec.Pipe, exec.PassThrough, exec.PassThrough)
	if err != nil {
		t.Fatal("cannot run compiler: ", err)
	}
	go func() {
		io.Copy(c.Stdin, bytes.NewBuffer(code))
		c.Stdin.Close()
	}()
	w, err := c.Wait(0)
	if err != nil {
		t.Fatal("wait error: ", err)
	}

	if w.ExitStatus() != 0 {
		t.Fatal("compile failed")
	}
}


func TestOneFile(t *testing.T) {
	code, offsetMap := translateSymbols(testCode)
	//fmt.Printf("------------------- {%s}\n", code)
	fset := token.NewFileSet()
	scope := ast.NewScope(parser.Universe)
	f, err := parser.ParseFile(fset, "xx.go", code, parser.DeclarationErrors, scope)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	v := make(identVisitor)
	go func() {
		ast.Walk(v, f)
		close(v)
	}()
	for e := range v {
		testExpr(t, fset, e, offsetMap)
	}
}

func testExpr(t *testing.T, fset *token.FileSet, e ast.Expr, offsetMap map[int]*sym) {
	var name *ast.Ident
	switch e := e.(type) {
	case *ast.SelectorExpr:
		name = e.Sel
	case *ast.Ident:
		name = e
	default:
		panic("unexpected expression type")
	}
	from := fset.Position(name.NamePos)
	obj, typ := ExprType(e)
	if obj == nil {
		t.Errorf("no object found for %v at %v", pretty{e}, from)
		return
	}
	if typ.Kind == ast.Bad {
		t.Errorf("no type found for %v at %v", pretty{e}, from)
		return
	}
	if name.Name != obj.Name {
		t.Errorf("wrong name found for %v at %v; expected %q got %q", pretty{e}, from, name, obj.Name)
		return
	}
	to := offsetMap[from.Offset]
	if to == nil {
		t.Errorf("no source symbol entered for %s at %v", name.Name, from)
		return
	}
	found := fset.Position(DeclPos(obj))
	if found.Offset != to.offset {
		t.Errorf("wrong offset found for %v at %v, decl %T (%#v); expected %d got %d", pretty{e}, from, obj.Decl, obj.Decl, to.offset, found.Offset)
	}
	if typ.Kind != to.kind {
		t.Errorf("wrong type for %s at %v; expected %v got %v", name.Name, from, to.kind, typ.Kind)
	}
}

type identVisitor chan ast.Expr

func (v identVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.Ident:
		if strings.HasPrefix(n.Name, prefix) {
			v <- n
		}
		return nil
	case *ast.SelectorExpr:
		ast.Walk(v, n.X)
		if strings.HasPrefix(n.Sel.Name, prefix) {
			v <- n
		}
		return nil
	}
	return v
}

const prefix = "xx"

var kinds = map[int]ast.ObjKind{
	'v': ast.Var,
	'c': ast.Con,
	't': ast.Typ,
	'f': ast.Fun,
}

type sym struct {
	name   string
	offset int
	kind   ast.ObjKind
}

// transateSymbols performs a non-parsing translation of some
// Go source code. For each symbol starting with xx,
// it returns an entry in offsetMap mapping
// from the reference in the source code to the first
// occurrence of that symbol.
// If the symbol is followed by #x, it refers
// to a particular version of the symbol. The
// translated code will produce only the bare
// symbol, but the expected symbol can be
// determined from the returned map.
//
// The first occurrence of a translated symbol must
// be followed by a letter representing the symbol
// kind (see kinds, above). All subsequent references
// to that symbol must resolve to the given kind.
//
func translateSymbols(code []byte) (result []byte, offsetMap map[int]*sym) {
	offsetMap = make(map[int]*sym)
	buf := bytes.NewBuffer(code)
	syms := make(map[string]*sym)
	var wbuf, sbuf bytes.Buffer
	for {
		r, _, err := buf.ReadRune()
		if err != nil {
			break
		}
		if r != int(prefix[0]) {
			wbuf.WriteRune(r)
			continue
		}
		sbuf.Reset()
		for unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '#' {
			sbuf.WriteRune(r)
			r, _, err = buf.ReadRune()
			if err != nil {
				break
			}
		}
		typec := 0
		if r == '@' {
			typec, _, err = buf.ReadRune()
		} else {
			buf.UnreadRune()
		}
		name := sbuf.String()
		if !strings.HasPrefix(name, prefix) {
			sbuf.WriteString(name)
			continue
		}
		bareName := name
		if i := strings.IndexRune(bareName, '#'); i >= 0 {
			bareName = bareName[:i]
		}
		s := syms[name]
		if s == nil {
			if typec == 0 {
				panic("first symbol reference must have type character")
			}
			s = &sym{name, wbuf.Len(), kinds[typec]}
			if s.kind == ast.Bad {
				panic("bad type character " + string(typec))
			}
			syms[name] = s
		}
		offsetMap[wbuf.Len()] = s
		wbuf.WriteString(bareName)
	}
	result = wbuf.Bytes()
	return
}

var testCode = []byte(
`package main

import "os"

type xx_struct@t struct {
	xx_1@v int
	xx_2@v int
}

type xx_link@t struct {
	xx_3@v    int
	xx_next@v *xx_link
}

type xx_structembed@t struct {
	xx_struct#f@v
}

type xx_interface@t interface {
	xx_value#i@f()
}

type xx_int@t int

func (xx_int) xx_k@f() {}

const (
	xx_inta@c, xx_int1@c = xx_int(iota), xx_int(iota * 2)
	xx_intb@c, xx_int2@c
	xx_intc@c, xx_int3@c
)

var fd1 = os.Stdin

func (xx_4@v *xx_struct) xx_ptr@f()  {
	_ = xx_4.xx_1
}
func (xx_5@v xx_struct) xx_value#s@f() {
	_ = xx_5.xx_2
}

func (s xx_structembed) xx_value#e@f() {}

type xx_other@t bool
func (xx_other) xx_value#x@f() {}

var xxv_int@v xx_int

var xx_chan@v chan xx_struct
var xx_map@v map[string]xx_struct

var (
	xx_func@v func() xx_struct
	xx_mvfunc@v func() (string, xx_struct, xx_struct)
	xxv_interface@v interface{}
)
var xxv_link@v *xx_link

func xx_foo@f(xx_int) xx_int {
	return 0
}

func main() {

	fd := os.NewFile(1, "/dev/stdout")
	_, _ = fd.Write(nil)
	fd1.Write(nil)

	_ = (<-xx_chan).xx_1

	_ = xx_map[""].xx_1

	xx_a2@v, _ := xx_map[""]
	_ = xx_a2.xx_2

	_ = xx_func().xx_1

	xx_c@v, xx_d@v, xx_e@v := xx_mvfunc()
	_ = xx_d.xx_2
	_ = xx_e.xx_1

	xx_f@v := func() xx_struct { return xx_struct{} }
	_ = xx_f().xx_2

	xx_g@v := xxv_interface.(xx_struct).xx_1
	xx_h@v, _ := xxv_interface.(xx_struct)
	_ = xx_h.xx_2

	var xx_6@v xx_interface = xx_struct{}

	switch xx_i@v := xx_6.(type) {
	case xx_struct, xx_structembed:
		xx_i.xx_value#i()
	case xx_interface:
		xx_i.xx_value#i()
	case xx_other:
		xx_i.xx_value#x()
	}

	xx_map2@v := make(map[xx_int]xx_struct)
	for xx_a@v, xx_b@v := range xx_map2 {
		xx_a.xx_k()
		_ = xx_b.xx_2
	}
	for xx_a3@v := range xx_map2 {
		xx_a3.xx_k()
	}

	for xx_a4@v := range xx_chan {
		_ = xx_a4.xx_1
	}

	xxv_struct@v := new(xx_struct)
	_ = xxv_struct.xx_1

	var xx_1e@v xx_structembed
	xx_1e.xx_value#e()
	xx_1e.xx_ptr()
	_ = xx_1e.xx_struct#f

	xxv_int.xx_k()
	xx_inta.xx_k()
	xx_intb.xx_k()
	xx_intc.xx_k()
	xx_int1.xx_k()
	xx_int2.xx_k()
	xx_int3.xx_k()

	xxa@v := []xx_int{1, 2, 3}
	xxa[0].xx_k()

	xxp@v := new(int)
	(*xx_int)(xxp).xx_k()

	xx_foo(5).xx_k()

	_ = xxv_link.xx_next.xx_next.xx_3

	use(xx_c, xx_d, xx_e, xx_f, xx_g, xx_h)
}

func use(...interface{}) {}
`)
