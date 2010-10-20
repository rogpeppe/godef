// The values package provides multiple-writer,
// multiple-listener access to changing values.
// It also provides (through the Transform function
// and the Lens type) the facility to have multiple,
// mutually updating views of the same value.
//
package values

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
//	CloseIter(c <-chan interface{})
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
	closed chan bool
}

func (r *reader) Close() {
	if r != nil {
		select {
		case r.closed <- true:
		default:
		}
	}
}

// NewValue creates a new Value with its
// initial value and type given by initial. If initial
// is nil, the type is taken to be interface{},
// and the initial value is
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
	}else{
		v.vtype = interfaceType
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
	if v.vtype != interfaceType && reflect.Typeof(val) != v.vtype {
		panic(fmt.Sprintf("wrong type set on Value[%v]: %T", v.vtype, val))
	}
	v.setc <- val
	return nil
}

func (v *value) Close() {
	close(v.setc)
}

func (v *value) Iter() <-chan interface{}  {
	out := make(chan interface{})
	closed := <-v.closed
	var r *reader
	if closed {
		close(out)
	} else {
		r = &reader{0, make(chan interface{}, 1), out, make(chan bool, 1)}
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

func (r *reader) sender(readyc chan<- *reader) {
loop:
	for {
		v := <-r.in
		if closed(r.in) {
			break loop
		}
		select{
		case r.out <- v:

		case <-r.closed:
			break loop
		}
		readyc <- r
	}
	close(r.out)
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
				// against race between Close and Iter.
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
