package canvas

import (
	"fmt"
	"os"
	"reflect"
)

// A Value represents a changing value of
// a given type.
// It can be watched for changes by receiving
// on a channel returned by Iter. If a reader
// is slow to read, it may miss intermediate
// values.
// Set changes the value; it never blocks.
// Close marks the channel as closed.
// Type returns the type associated with the Value.
// Note that watchers should not change
// a Value in response to a value received from
// that channel - this could lead to an infinite
// loop.
//
type Value interface {
	Set(val interface{}) os.Error
	Iter() <-chan interface{}
	Close()
	Type() reflect.Type
}

// always accept a value from a Value channel - never
// Set in response to a receive.
//
type value struct {
	readyc chan *reader
	setc   chan interface{}
	vtype  reflect.Type

	// closed contains a single value which is true
	// when Close has been called on the Value
	// and the receiver goroutine has exited.
	closed chan bool
}

type reader struct {
	version int
	in      chan interface{}
	out     chan<- interface{}
}

// NewValue creates a new Value with its
// initial value and type given by initial. If initial
// is nil, any type is allowed, and the initial value is
// not set.
//
func NewValue(initial interface{}) Value {
	v := new(value)
	v.readyc = make(chan *reader)
	v.setc = make(chan interface{})
	version := 0
	if initial != nil {
		v.vtype = reflect.Typeof(initial)
		version++
	}
	v.closed = make(chan bool, 1)
	v.closed <- false
	go v.receiver(initial, version)
	return v
}

func (v *value) Type() reflect.Type {
	return v.vtype
}

func (v *value) Set(val interface{}) os.Error {
	if v.vtype != nil && reflect.Typeof(val) != v.vtype {
		panic(fmt.Sprintf("wrong type set on Value[%v]: %T", v.vtype, val))
	}
	v.setc <- val
	return nil
}

func (v *value) Close() {
	close(v.setc)
}

func (v *value) Iter() <-chan interface{} {
	out := make(chan interface{})
	closed := <-v.closed
	if closed {
		close(out)
	} else {
		r := &reader{0, make(chan interface{}), out}
		go r.sender(v.readyc)

		// send the first ready signal synchronously,
		// so we know that the value hasn't been
		// Closed between starting the sender
		// and it sending on readyc.
		v.readyc <- r
	}
	v.closed <- closed
	return out
}

func (v *value) receiver(val interface{}, version int) {
	ready := make([]*reader, 0, 2)

	for {
		select {
		case nval := <-v.setc:
			if closed(v.setc) {
				// to close, we first notify all known readers,
				// then we set closed to true, acknowledging
				// new readers at the same time, to guard
				// against race between Close and Get.
				for _, r := range ready {
					close(r.in)
				}
				for {
					select {
					case <-v.closed:
						v.closed <- true
						return
					case r := <-v.readyc:
						close(r.in)
					}
				}
			}

			version++
			for _, r := range ready {
				r.in <- nval
				r.version = version
			}
			val = nval
			ready = ready[0:0]

		case r := <-v.readyc:
			if r.version == version {
				if len(ready) == cap(ready) {
					nr := make([]*reader, len(ready), cap(ready)+2)
					copy(nr, ready)
					ready = nr
				}
				ready = ready[0 : len(ready)+1]
				ready[len(ready)-1] = r
			} else {
				r.in <- val
				r.version = version
			}
		}
	}
}

func (r *reader) sender(readyc chan<- *reader) {
	for {
		v := <-r.in
		if closed(r.in) {
			close(r.out)
			break
		}
		r.out <- v
		readyc <- r
	}
}

// A Mirror can transform between values of two
// different types.
type Mirror struct {
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
				panic("invalid type passed to Mirror.Transform")
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

// NewMirror creates a new Mirror instance.
// Both f and finv must be functions; their
// actual signatures must be:
// f func(T) (T1, os.Error)
// finv func(T1) (T, os.Error)
// for some types T and T1.
// finv is expected to be the inverse of f;
// if either function receives a value that cannot
// be successfully converted, it should return
// a non-nil os.Error.
//
func NewMirror(f, finv interface{}) *Mirror {
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
	return &Mirror{
		caller(reflect.NewValue(f).(*reflect.FuncValue), t),
		caller(reflect.NewValue(finv).(*reflect.FuncValue), t1),
		t,
		t1,
	}
}

func (m *Mirror) Reverse() *Mirror {
	return &Mirror{m.finv, m.f, m.t1, m.t}
}

// layer m1 on top of m
func (m *Mirror) Combine(m1 *Mirror) *Mirror {
	return &Mirror{
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

func (m *Mirror) Transform(val interface{}) (interface{}, os.Error) {
	return m.f(val)
}

func (m *Mirror) Type() reflect.Type {
	return m.t
}

func (m *Mirror) Type1() reflect.Type {
	return m.t1
}

type transformedValue struct {
	v Value
	m *Mirror
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
func Transform(v Value, m *Mirror) (v1 Value) {
	if v.Type() != m.Type() {
		panic(fmt.Sprintf("Value type (%v) does not match Mirror type (%v)", v.Type(), m.Type()))
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


func Float2String(printf, scanf string) *Mirror {
	// do early sanity check on format.
	s := fmt.Sprintf(printf, float64(0))
	var f float64
	_, err := fmt.Sscanf(s, scanf, &f)
	if err != nil || f != 0 {
		panic(fmt.Sprintf("non-reversible format %#v<->%#v (got %#v), err %v", printf, scanf, s, err))
	}
	return NewMirror(
		func(f float64) (string, os.Error) {
			return fmt.Sprintf(printf, f), nil
		},
		func(s string) (float64, os.Error) {
			var f float64
			_, err := fmt.Sscanf(s, scanf, &f)
			return f, err
		},
	)
}

func UnitFloat2RangedFloat(lo, hi float64) *Mirror {
	return NewMirror(
		func(uf float64) (float64, os.Error) {
			if uf > 1 {
				return 0, os.NewError("value too high")
			}
			if uf < 0 {
				return 0, os.NewError("value too low")
			}
			return lo + (uf * (hi - lo)), nil
		},
		func(rf float64) (float64, os.Error) {
			if rf > hi {
				return 0, os.NewError("value too high")
			}
			if rf < lo {
				return 0, os.NewError("value too low")
			}
			return (rf - lo) / (hi - lo), nil
		},
	)
}

func FloatMultiply(x float64) *Mirror {
	return NewMirror(
		func(f float64) (float64, os.Error) {
			return f * x, nil
		},
		func(rf float64) (float64, os.Error) {
			return rf / x, nil
		},
	)
}
