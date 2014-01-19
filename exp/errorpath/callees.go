package main

import (
	"fmt"
	"go/token"
	"reflect"
	"unsafe"

	"code.google.com/p/go.tools/go/ssa"
	"code.google.com/p/go.tools/oracle"
)

func (ctxt *context) callees(inst *ssa.Call) ([]*ssa.Function, error) {
	pos := ctxt.lprog.Fset.Position(inst.Pos())
	if pos.Line <= 0 {
		return nil, fmt.Errorf("no position")
	}
	qpos, err := oracle.ParseQueryPos(ctxt.lprog, posStr(pos), true)
	if err != nil {
		return nil, fmt.Errorf("cannot parse query pos %q: %v", posStr(pos), err)
	}
	result, err := ctxt.oracle.Query("callees", qpos)
	if err != nil {
		return nil, fmt.Errorf("query error: %v", err)
	}
	return calleeFuncs(result), nil
}

func calleeFuncs(r *oracle.Result) []*ssa.Function {
	if r == nil {
		return nil
	}
	v := reflect.ValueOf(r).Elem()
	if v.FieldByName("mode").String() != "callees" {
		panic("not callees result")
	}
	funcs := v.FieldByName("q").Elem().Elem().FieldByName("funcs")
	return bypassCanInterface(funcs).Interface().([]*ssa.Function)
}

func posStr(pos token.Position) string {
	return fmt.Sprintf("%s:#%d", pos.Filename, pos.Offset)
}

type reflectFlag uintptr

// copied from reflect/value.go
const (
	flagRO reflectFlag = 1 << iota
)

var flagValOffset = func() uintptr {
	field, ok := reflect.TypeOf(reflect.Value{}).FieldByName("flag")
	if !ok {
		panic("reflect.Value has no flag field")
	}
	return field.Offset
}()

func flagField(v *reflect.Value) *reflectFlag {
	return (*reflectFlag)(unsafe.Pointer(uintptr(unsafe.Pointer(v)) + flagValOffset))
}

// bypassCanInterface returns a version of v that
// bypasses the CanInterface check.
func bypassCanInterface(v reflect.Value) reflect.Value {
	if !v.IsValid() || v.CanInterface() {
		return v
	}
	*flagField(&v) &^= flagRO
	return v
}

// Sanity checks against future reflect package changes
// to the type or semantics of the Value.flag field.
func init() {
	field, ok := reflect.TypeOf(reflect.Value{}).FieldByName("flag")
	if !ok {
		panic("reflect.Value has no flag field")
	}
	if field.Type.Kind() != reflect.TypeOf(reflectFlag(0)).Kind() {
		panic("reflect.Value flag field has changed kind")
	}
	var t struct {
		a int
		A int
	}
	vA := reflect.ValueOf(t).FieldByName("A")
	va := reflect.ValueOf(t).FieldByName("a")
	flagA := *flagField(&vA)
	flaga := *flagField(&va)
	if flagA&flagRO != 0 || flaga&flagRO == 0 {
		panic("reflect.Value read-only flag has changed value")
	}
}
