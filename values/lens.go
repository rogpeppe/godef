package values
import (
	"fmt"
	"os"
	"reflect"
)

// A Lens can transform from values of type T to values
// of type T1. It can be reversed to transform in the
// other direction, and be combined with other Lenses.
//
type Lens struct {
	f, finv func(interface{}) (interface{}, os.Error)
	t, t1   reflect.Type
}

func caller(f *reflect.FuncValue, argt reflect.Type) func(interface{}) (interface{}, os.Error) {
	_, isInterface := argt.(*reflect.InterfaceType)
	return func(arg interface{}) (interface{}, os.Error) {
		argv := reflect.NewValue(arg)
		// if T is an interface type, then we need to
		// explicitly perform the type conversion,
		// otherwise the static types will not match.
		if argv.Type() != argt {
			if isInterface {
				i := reflect.MakeZero(argt).(*reflect.InterfaceValue)
				i.Set(argv)
				argv = i
			} else {
				panic("invalid type passed to Lens.Transform")
			}
		}
		t1val := f.Call([]reflect.Value{argv})
		var err os.Error
		if e := t1val[1].Interface(); e != nil {
			err = e.(os.Error)
		}
		return t1val[0].Interface(), err
	}
}

// NewLens creates a new Lens instance.
// Both f and finv must be functions; their
// actual signatures must be:
// f func(T) (T1, os.Error)
// finv func(T1) (T, os.Error)
// for some types T and T1.
// finv is expected to be the inverse of f.
// If either function receives a value that cannot
// be successfully converted, it should return
// a non-nil os.Error.
//
func NewLens(f, finv interface{}) *Lens {
	ft := reflect.Typeof(f).(*reflect.FuncType)
	finvt := reflect.Typeof(finv).(*reflect.FuncType)

	if ft.NumIn() != 1 || ft.NumOut() != 2 ||
		finvt.NumIn() != 1 || finvt.NumOut() != 2 ||
		ft.In(0) != finvt.Out(0) ||
		ft.Out(1) != osErrorType ||
		finvt.In(0) != ft.Out(0) ||
		finvt.Out(1) != osErrorType {
		panic(fmt.Sprintf("bad transform function types: %T; %T", f, finv))
	}
	t := ft.In(0)
	t1 := ft.Out(0)
	return &Lens{
		caller(reflect.NewValue(f).(*reflect.FuncValue), t),
		caller(reflect.NewValue(finv).(*reflect.FuncValue), t1),
		t,
		t1,
	}
}

// Reverse returns a lens that transforms in the opposite
// direction to m.
func (m *Lens) Reverse() *Lens {
	return &Lens{m.finv, m.f, m.t1, m.t}
}

// Combine layers m1 on top of m.
func (m *Lens) Combine(m1 *Lens) *Lens {
	return &Lens{
		func(x interface{}) (interface{}, os.Error) {
			x1, err := m.f(x)
			if err != nil {
				return nil, err
			}
			return m1.f(x1)
		},
		func(x2 interface{}) (interface{}, os.Error) {
			x1, err := m1.finv(x2)
			if err != nil {
				return nil, err
			}
			return m.finv(x1)
		},
		m1.t,
		m.t1,
	}
}

// Transform transforms a value of type T into a value of type T1.
// 
func (m *Lens) Transform(val interface{}) (interface{}, os.Error) {
	return m.f(val)
}

// Type returns the type of T.
func (m *Lens) Type() reflect.Type {
	return m.t
}

// Type1 returns the type of T1.
func (m *Lens) Type1() reflect.Type {
	return m.t1
}

type transformedValue struct {
	v Value
	m *Lens
}

// XXX there must be a simpler way to do this!
var osErrorType = reflect.Typeof(func(os.Error) {}).(*reflect.FuncType).In(0)

// Transform returns a Value, v1, that mirrors an existing Value, v,
// by running m.Transform(x) on each value received from
// v.Iter(), and m.Reverse.Transform(x) on each value
// passed to v.Set().
// m.Type must equal v.Type.
// The Type of the resulting value is m.Type1().
//
func Transform(v Value, m *Lens) (v1 Value) {
	if v.Type() != m.Type() {
		panic(fmt.Sprintf("Value type (%v) does not match Lens type (%v)", v.Type(), m.Type()))
	}
	return transformedValue{v, m}
}

func (v transformedValue) Close() {
	v.v.Close()
}

func (v transformedValue) Iter() <-chan interface{} {
	c := make(chan interface{})
	go func() {
		for x := range v.v.Iter() {
			x1, err := v.m.Transform(x)
			if err != nil {
				// could log a message, but discarding the
				// bad values might also be ok
			} else {
				c <- x1
			}
		}
	}()
	return c
}

func (v transformedValue) Type() reflect.Type {
	return v.m.Type1()
}

func (v transformedValue) Set(x1 interface{}) os.Error {
	x, err := v.m.Reverse().Transform(x1)
	if err != nil {
		return err
	}
	v.v.Set(x)
	return nil
}
