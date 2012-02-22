package values

import (
	"errors"
	"reflect"
)

// NewConst returns a Value of type t which always returns the value v,
// and gives an error when set.
func NewConst(val interface{}, t reflect.Type) Value {
	if t == nil {
		return &constValue{reflect.ValueOf(val)}
	}
	v := &constValue{reflect.New(t).Elem()}
	v.val.Set(reflect.ValueOf(val))
	return v
}

type constValue struct {
	val reflect.Value
}

func (v *constValue) Get() (x interface{}, ok bool) {
	return v.val.Interface(), false
}

func (v *constValue) Getter() Getter {
	return &constGetter{v.val, false}
}

func (v *constValue) Type() reflect.Type {
	return v.val.Type()
}

func (v *constValue) Set(_ interface{}) error {
	return errors.New("cannot set constant value")
}

func (v *constValue) Close() error {
	return errors.New("const is already closed")
}

type constGetter struct {
	val  reflect.Value
	done bool
}

func (v *constGetter) Type() reflect.Type {
	return v.val.Type()
}

func (v *constGetter) Get() (x interface{}, ok bool) {
	if v.done {
		return nil, false
	}
	v.done = true
	return v.val.Interface(), true
}
