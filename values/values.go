// The values package provides multiple-writer,
// multiple-listener access to changing values.
// It also provides (through the Transform function
// and the Lens type) the facility to have multiple,
// mutually updating views of the same value.
//
package values

import (
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
	Set(val interface{}) error

	// Getter returns a Getter that can be used to listen
	// for changes to the value.
	Getter() Getter

	// Get gets the most recently set value. If the Value
	// has been Closed, ok will be false, but, unlike channels,
	// the value will still be the last set value.
	Get() (x interface{}, ok bool)

	// Type returns the type associated with the Value.
	Type() reflect.Type

	// Close marks the value as closed; all blocked Getters
	// will return.
	Close() error
}

type Getter interface {
	// Get gets the most recent value. If the Value has not been Set
	// since Get was last called, it blocks until it is.
	// When the Value has been closed and the final
	// value has been got, ok will be false and x nil.
	Get() (x interface{}, ok bool)

	// Type returns the type associated with the Getter (and its Value)
	Type() reflect.Type
}

type value struct {
	mu      sync.Mutex
	wait    sync.Cond
	val     reflect.Value
	version int
	closed  bool
}

type getter struct {
	v       *value
	version int
}

// NewValue creates a new Value with the
// given initial value and type.
// If t is nil, the type will be taken from initial;
// if initial is also nil, the type will be interface{}.
// If initial is nil, any Getter will block until
// a value is first set.
func NewValue(initial interface{}, t reflect.Type) Value {
	v := new(value)
	v.wait.L = &v.mu
	if t == nil {
		if initial != nil {
			t = reflect.TypeOf(initial)
		} else {
			t = interfaceType
		}
	}
	v.val = reflect.New(t).Elem()
	if initial != nil {
		v.val.Set(reflect.ValueOf(initial))
		v.version++
	}
	return v
}

func (v *value) Type() reflect.Type {
	return v.val.Type()
}

func (v *value) Set(val interface{}) error {
	v.mu.Lock()
	v.val.Set(reflect.ValueOf(val))
	v.version++
	v.mu.Unlock()
	v.wait.Broadcast()
	return nil
}

func (v *value) Close() error {
	v.mu.Lock()
	v.closed = true
	v.mu.Unlock()
	v.wait.Broadcast()
	return nil
}

func (v *value) Get() (interface{}, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.val.Interface(), !v.closed
}

func (v *value) Getter() Getter {
	return &getter{v: v}
}

func (g *getter) Get() (interface{}, bool) {
	g.v.mu.Lock()
	defer g.v.mu.Unlock()

	// We should never go around this loop more than twice.
	for {
		if g.version != g.v.version {
			g.version = g.v.version
			return g.v.val.Interface(), true
		}
		if g.v.closed {
			return nil, false
		}
		g.v.wait.Wait()
	}
	panic("not reached")
}

func (g *getter) Type() reflect.Type {
	return g.v.Type()
}

// Sender sends values from v down
// the channel c, which must be of type T
// where T is v.Type().
// When the Value is closed, the channel
// is closed.
func Sender(v Value, c interface{}) {
	cv := reflect.ValueOf(c)
	if cv.Kind() != reflect.Chan || cv.Type().Elem() != v.Type() {
		panic("expected chan " + v.Type().String() + "; got " + cv.Type().String())
	}
	g := v.Getter()
	for {
		x, ok := g.Get()
		if !ok {
			break
		}
		cv.Send(reflect.ValueOf(x))
	}
	cv.Close()
}
