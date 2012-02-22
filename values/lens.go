package values

import (
	"fmt"
	"reflect"
)

// A Lens can transform from values of type T to values
// of type T1. It can be reversed to transform in the
// other direction, and be combined with other Lenses.
//
type Lens struct {
	f, finv func(reflect.Value) (reflect.Value, error)
	t, t1   reflect.Type
}

func okTransform(f reflect.Type) bool {
	return f.Kind() == reflect.Func &&
		f.NumIn() == 1 &&
		f.NumOut() == 2 &&
		f.Out(1) == osErrorType
}

// NewLens creates a new Lens instance that transforms values
// from type T to type T1. Both f and finv must be functions; their
// actual signatures must be:
// f func(T) (T1, os.Error)
// finv func(T1) (T, os.Error)
// for some types T and T1.
// finv is expected to be the inverse of f.
// If either function receives a value that cannot be successfully converted,
// it should return an error.
func NewLens(f, finv interface{}) *Lens {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	finvv := reflect.ValueOf(finv)
	finvt := finvv.Type()

	if !okTransform(ft) ||
		!okTransform(finvt) ||
		ft.In(0) != finvt.Out(0) ||
		finvt.In(0) != ft.Out(0) {
		panic(fmt.Sprintf("bad transform function types: %T; %T", f, finv))
	}
	return &Lens{
		caller(fv),
		caller(finvv),
		ft.In(0),
		ft.Out(0),
	}
}

// NewReflectiveLens creates a Lens from two dynamically typed functions.
// f should convert from type t to type t1;
// finv should convert in the other direction.
// The uses for this function are fairly esoteric, and can
// be used to break the type safety of the Value if used without care.
func NewReflectiveLens(f, finv func(reflect.Value) (reflect.Value, error), t, t1 reflect.Type) *Lens {
	return &Lens{f, finv, t, t1}
}

// caller converts from an actual function to a function type
// that we can construct on the fly.
func caller(f reflect.Value) func(reflect.Value) (reflect.Value, error) {
	return func(v reflect.Value) (reflect.Value, error) {
		r := f.Call([]reflect.Value{v})
		if r[1].IsNil() {
			return r[0], nil
		}
		return r[0], r[1].Interface().(error)
	}
}

// Reverse returns a lens that transforms in the opposite
// direction to m, from m.Type1() to m.Type()
func (m *Lens) Reverse() *Lens {
	return &Lens{m.finv, m.f, m.t1, m.t}
}

// Combine layers m1 on top of m. If m transforms from
// type T to T1 and m1 transforms from type T1 to T2,
// then the returned Lens transforms from T to T2.
// Combine panics if m.Type1() != m1.Type().
func (m *Lens) Combine(m1 *Lens) *Lens {
	if m.Type1() != m1.Type() {
		panic("incompatible Lens combination")
	}
	return &Lens{
		func(x reflect.Value) (reflect.Value, error) {
			x1, err := m.f(x)
			if err != nil {
				return x1, err
			}
			return m1.f(x1)
		},
		func(x2 reflect.Value) (reflect.Value, error) {
			x1, err := m1.finv(x2)
			if err != nil {
				return x1, err
			}
			return m.finv(x1)
		},
		m1.t,
		m.t1,
	}
}

// Transform transforms a value of type T into a value of type T1.
// The value val must be assignable to T.
func (m *Lens) Transform(val interface{}) (interface{}, error) {
	x, err := m.f(reflect.ValueOf(val))
	if err != nil {
		return nil, err
	}
	return x.Interface(), nil
}

// Type returns the type of T - the type that
// the Lens transforms to.
func (m *Lens) Type() reflect.Type {
	return m.t
}

// Type1 returns the type of T1 - the type
// that the Lens transforms from.
func (m *Lens) Type1() reflect.Type {
	return m.t1
}

type transformedValue struct {
	v Value
	m *Lens
}

type transformedGetter struct {
	g Getter
	m *Lens
}

var osErrorType = reflect.TypeOf((*error)(nil)).Elem()
var interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()

// Transform returns a Value, v1, that mirrors an existing Value, v,
// by running m.Transform(x) on each value received from
// v.Iter(), and m.Reverse.Transform(x) on each value passed to v.Set().
// m.Type() must equal v.Type().
// The Type of the resulting value is m.Type1().
//
func Transform(v Value, m *Lens) (v1 Value) {
	if v.Type() != m.Type() {
		panic(fmt.Sprintf("Value type (%v) does not match Lens type (%v)", v.Type(), m.Type()))
	}
	return &transformedValue{v, m}
}

func (v *transformedValue) Get() (interface{}, bool) {
	x, ok := v.v.Get()
	// TODO what should we do with an error here?
	x1, _ := v.m.Transform(x)
	return x1, ok
}

func (v *transformedValue) Type() reflect.Type {
	return v.m.Type1()
}

func (v *transformedValue) Close() error {
	return v.v.Close()
}

func (v *transformedValue) Set(x1 interface{}) error {
	x, err := v.m.Reverse().Transform(x1)
	if err != nil {
		return err
	}
	return v.v.Set(x)
}

func (v *transformedValue) Getter() Getter {
	return &transformedGetter{v.v.Getter(), v.m}
}

func (g *transformedGetter) Get() (interface{}, bool) {
	// loop until we get a valid value.
	for {
		x, ok := g.g.Get()
		x1, err := g.m.Transform(x)
		if err == nil || !ok {
			return x1, ok
		}
	}
	panic("not reached")
}

func (g *transformedGetter) Type() reflect.Type {
	return g.m.Type1()
}
