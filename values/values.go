// The values package provides multiple-writer,
// multiple-listener access to changing values.
// It also provides (through the Transform function
// and the Lens type) the facility to have multiple,
// mutually updating views of the same value.
//
package values

import (
	"os"
	"reflect"
	"sync"
)

// A Value represents a changing value of a given type.  Note that
// watchers should not change a Value in response to a value received
// from that channel - this could lead to an infinite loop.
//
type Value interface {
	// Set changes the value. It never blocks; it will panic if
	// the value is not assignable to the Value's type.
	Set(val interface{}) os.Error

	// Getter returns a Getter that can be used to listen
	// for changes to the value.
	Getter() Getter

	// Get gets the most recently set value.
	Get() interface{}

	// Type returns the type associated with the Value.
	Type() reflect.Type
}

type Getter interface {
	// Get gets the most recent value. If the value has not been Set
	// since Get was last called, it blocks until it is.
	Get() interface{}

	// Type returns the type associated with the Getter (and its Value)
	Type() reflect.Type
}

type value struct {
	mu sync.Mutex
	wait sync.Cond
	val reflect.Value
	version int
}

type getter struct {
	v *value
	version int
}

// NewValue creates a new Value with its
// initial value and type given by initial. If initial
// is nil, the type is taken to be interface{},
// and the initial value is not set.
//
func NewValue(initial interface{}) Value {
	val := reflect.ValueOf(initial)
	v := newValue(val.Type())
	v.version = 1
	v.val.Set(val)
	return v
}

func NewValueWithType(t reflect.Type) Value {
	v := newValue(t)
	v.version = 0
	return v
}

func newValue(t reflect.Type) *value {
	v := &value{
		val: reflect.New(t).Elem(),
	}
	v.wait.L = &v.mu
	return v
}

func (v *value) Type() reflect.Type {
	return v.val.Type()
}

func (v *value) Set(val interface{}) os.Error {
	v.mu.Lock()
	v.val.Set(reflect.ValueOf(val))
	v.version++
	v.mu.Unlock()
	v.wait.Broadcast()
	return nil
}

func (v *value) Get() interface{} {
	v.mu.Lock()
	val := v.val
	v.mu.Unlock()
	return val.Interface()
}

func (v *value) Getter() Getter {
	return &getter{v: v}
}

func (g *getter) Get() interface{} {
	v := g.v
	v.mu.Lock()
	if g.version == v.version {
		v.wait.Wait()
	}
	val := v.val
	g.version = v.version
	v.mu.Unlock()
	return val.Interface()
}

func (g *getter) Type() reflect.Type {
	return g.v.Type()
}
