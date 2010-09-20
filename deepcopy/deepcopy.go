// The deepcopy package implements deep copying of arbitrary
// data structures, making sure that self references and shared pointers
// are preserved.
package deepcopy

import (
	"fmt"
	"reflect"
	"unsafe"
	"sync"
)

// basic copy algorithm:
// 1) recursively scan object, building up a list of all allocated
// memory ranges pointed to by the object.
// 2) recursively copy object, making sure that references
// within the data structure point to the appropriate
// places in the newly allocated data structure.

type memRange struct {
	m0, m1 uintptr
}

type memRangeList struct {
	memRange
	t         reflect.Type                                                          // type to be passed to make.
	v         reflect.Value                                                         // newly allocated value.
	allocAddr uintptr                                                               // address of newly allocated value.
	copied    bool                                                                  // has this range been copied yet?
	make      func(r memRange, t reflect.Type) (v reflect.Value, allocAddr uintptr) // allocate space for this range.
	next      *memRangeList
}

type memRanges struct {
	l *memRangeList
}

// two functions are required to copy a given type:
//   ranges traverses the type, adding and memory segments
//	found to m.
//  copy makes a copy of the given type, storing the result in dst.
//
type copyFuncs struct {
	ranges func(obj reflect.Value, f *funcStore, m *memRanges)
	copy   func(dst, obj reflect.Value, f *funcStore, m *memRanges)
}

var funcMap = make(map[reflect.Type]*copyFuncs)
var lock sync.Mutex

// funcStore is essentially just a single map, but
// is stored as two so that new types can be added
// to the map concurrently without placing a lock around every
// map access. This works because the same functions
// will be generated for any given type.
//
type funcStore struct {
	funcs, nfuncs map[reflect.Type]*copyFuncs
}

// Copy makes a recursive deep copy of obj and returns the result.
//
// Pointer equality between items within obj is preserved,
// as are the relationships between slices that point to the same underlying data,
// although the data itself will be copied.
// a := Copy(b) implies reflect.DeepEqual(a, b).
// Map keys are not copied, as reflect.DeepEqual does not
// recurse into map keys.
// Due to restrictions in the reflect package, only
// types with all public members may be copied.
//
func Copy(obj interface{}) (r interface{}) {
	var m memRanges
	v := reflect.NewValue(obj)

	lock.Lock()
	funcs := funcStore{funcMap, nil}
	f := deepCopyFuncs(&funcs, v.Type())
	lock.Unlock()

	if f.ranges != nil {
		f.ranges(v, &funcs, &m)
	}
	dst := reflect.MakeZero(v.Type())
	f.copy(dst, v, &funcs, &m)

	// If we have encountered some new types while traversing the
	// data structure, add all the original types back into the map
	// and set the map to the new map.
	// This means that several goroutines can be using
	// the func map concurrently without acquiring the lock
	// for each access.
	// As the overall number of types is likely to be relatively
	// small and quickly asymptotic, the overhead of copying the
	// entire map each time should be negligible.
	// It doesn't matter if several goroutines add the same
	// types, because they will all have identical functions.
	if funcs.nfuncs != nil {
		lock.Lock()
		for k, v := range funcMap {
			funcs.nfuncs[k] = v
		}
		funcMap = funcs.nfuncs
		lock.Unlock()
	}
	return dst.Interface()
}

// deepCopyFuncs returns the copyFuncs for the given type t,
// To save recursively inspecting its type each time an object
// is copied, we store (in funcs) functions tailored to each type that
// know how to calculate ranges and copy objects of that type.
//
func deepCopyFuncs(funcs *funcStore, t reflect.Type) (f *copyFuncs) {
	if f = funcs.get(t); f != nil {
		return
	}
	// must add to funcs before switch so that recursive types work. the range
	// function is only inspected for whether it is non-nil - all recursive
	// types should have a non-nil ranges function, so the recursion
	// will stop.
	f = &copyFuncs{dummyRanges, nil}
	funcs.set(t, f)
	switch t := t.(type) {
	case *reflect.InterfaceType:
		f.ranges = func(obj0 reflect.Value, funcs *funcStore, m *memRanges) {
			obj := obj0.(*reflect.InterfaceValue)
			e := obj.Elem()
			if efns := deepCopyFuncs(funcs, e.Type()); efns.ranges != nil {
				efns.ranges(e, funcs, m)
			}
		}
		f.copy = func(dst, obj0 reflect.Value, funcs *funcStore, m *memRanges) {
			e := obj0.(*reflect.InterfaceValue).Elem()
			efns := deepCopyFuncs(funcs, e.Type())

			// we cannot just pass dst to efns.copy() because
			// various types (e.g. arrays and structs) expect an 
			// actual instance of the type to modify
			v := reflect.MakeZero(e.Type())
			efns.copy(v, e, funcs, m)
			dst.SetValue(v)
		}

	case *reflect.MapType:
		et := t.Elem()
		efns := deepCopyFuncs(funcs, et)
		f.ranges = func(obj0 reflect.Value, funcs *funcStore, m *memRanges) {
			obj := obj0.(*reflect.MapValue)
			m0 := obj.Get()
			if m.add(memRange{m0, m0 + 1}, t, nil) {
				if efns.ranges != nil {
					for _, k := range obj.Keys() {
						efns.ranges(obj.Elem(k), funcs, m)
					}
				}
			}
		}
		f.copy = func(dst, obj0 reflect.Value, funcs *funcStore, m *memRanges) {
			obj := obj0.(*reflect.MapValue)
			m0 := obj.Get()
			e := m.get(m0)
			if e == nil {
				panic("map range not found")
			}
			if !e.copied {
				// we don't copy the map keys because reflect.deepEqual
				// doesn't do deep equality on map keys.
				e.copied = true
				v := reflect.MakeMap(t)
				e.v = v
				dst.SetValue(e.v)
				if efns.ranges != nil {
					kv := reflect.MakeZero(t.Elem())
					for _, k := range obj.Keys() {
						efns.copy(kv, obj.Elem(k), funcs, m)
						v.SetElem(k, kv)
					}
				} else {
					for _, k := range obj.Keys() {
						v.SetElem(k, obj.Elem(k))
					}
				}
			} else {
				dst.SetValue(e.v)
			}
		}

	case *reflect.PtrType:
		et := t.Elem()
		efns := deepCopyFuncs(funcs, et)
		esize := et.Size()
		f.ranges = func(obj0 reflect.Value, funcs *funcStore, m *memRanges) {
			obj := obj0.(*reflect.PtrValue)
			if !obj.IsNil() {
				m0 := obj.Get()
				if m.add(memRange{m0, m0 + esize}, et, makeZero) {
					if efns.ranges != nil {
						efns.ranges(obj.Elem(), funcs, m)
					}
				}
			}
		}
		f.copy = func(dst0, obj0 reflect.Value, funcs *funcStore, m *memRanges) {
			obj := obj0.(*reflect.PtrValue)
			if obj.IsNil() {
				return
			}
			m0 := obj.Get()
			m1 := m0 + esize
			e := m.get(m0)
			if e == nil {
				panic("range not found")
			}
			if e.v == nil {
				e.v, e.allocAddr = e.make(e.memRange, e.t)
			}
			ptr := m0 - e.m0 + e.allocAddr
			v := reflect.NewValue(unsafe.Unreflect(t, unsafe.Pointer(&ptr))).(*reflect.PtrValue)
			// make a copy only if we're pointing to the entire
			// allocated object; otherwise the copy will be made
			// later when we get to the pointer that does point
			// to it.
			if m0 == e.m0 && m1 == e.m1 && !e.copied {
				e.copied = true
				efns.copy(v.Elem(), obj.Elem(), funcs, m)
			}
			dst0.SetValue(v)
		}

	case *reflect.ArrayType:
		efns := deepCopyFuncs(funcs, t.Elem())
		n := t.Len()
		if efns.ranges != nil {
			f.ranges = func(obj0 reflect.Value, funcs *funcStore, m *memRanges) {
				obj := obj0.(*reflect.ArrayValue)
				for i := 0; i < n; i++ {
					efns.ranges(obj.Elem(i), funcs, m)
				}
			}
			f.copy = func(dst0, obj0 reflect.Value, funcs *funcStore, m *memRanges) {
				dst := dst0.(*reflect.ArrayValue)
				obj := obj0.(*reflect.ArrayValue)
				for i := 0; i < n; i++ {
					efns.copy(dst.Elem(i), obj.Elem(i), funcs, m)
				}
			}
		} else {
			f.ranges = nil
			f.copy = func(dst0, obj0 reflect.Value, funcs *funcStore, m *memRanges) {
				dst := dst0.(*reflect.ArrayValue)
				obj := obj0.(*reflect.ArrayValue)
				reflect.ArrayCopy(dst, obj)
			}
		}

	case *reflect.SliceType:
		et := t.Elem()
		esize := et.Size()
		efns := deepCopyFuncs(funcs, et)
		f.ranges = func(obj0 reflect.Value, funcs *funcStore, m *memRanges) {
			obj := obj0.(*reflect.SliceValue)
			if obj.IsNil() {
				return
			}
			n := obj.Cap()
			m1 := obj.Get()
			m0 := m1 - uintptr(n)*esize
			if m.add(memRange{m0, m1}, obj.Type(), makeSlice) {
				if efns.ranges != nil {
					obj = obj.Slice(0, n)
					for i := 0; i < n; i++ {
						efns.ranges(obj.Elem(i), funcs, m)
					}
				}
			}
		}
		f.copy = func(dst0, obj0 reflect.Value, funcs *funcStore, m *memRanges) {
			obj := obj0.(*reflect.SliceValue)
			if obj.IsNil() {
				return
			}
			dst := dst0.(*reflect.SliceValue)
			cap := obj.Cap()
			m1 := obj.Get()
			m0 := m1 - uintptr(obj.Cap())*esize
			e := m.get(m0)
			if e == nil {
				panic("slice range not found")
			}
			// allocate whole object, even if obj represents just a part of it
			if e.v == nil {
				e.v, e.allocAddr = e.make(e.memRange, e.t)
			}
			// we must fabricate the slice, as we may be pointing
			// to a slice of an in-struct array, rather than another slice.
			h := reflect.SliceHeader{m0 - e.m0 + e.allocAddr, obj.Len(), obj.Cap()}
			dst.SetValue(reflect.NewValue(unsafe.Unreflect(t, unsafe.Pointer(&h))))
			if e.m0 == m0 && e.m1 == m1 && !e.copied {
				e.copied = true
				obj := obj.Slice(0, cap)
				dst := dst.Slice(0, cap)
				if efns.ranges != nil {
					for i := 0; i < cap; i++ {
						efns.copy(dst.Elem(i), obj.Elem(i), funcs, m)
					}
				} else {
					reflect.ArrayCopy(dst, obj)
				}
			}
		}

	case *reflect.StructType:
		efns := make([]*copyFuncs, t.NumField())
		hasPointers := false
		for i := 0; i < t.NumField(); i++ {
			efns[i] = deepCopyFuncs(funcs, t.Field(i).Type)
			hasPointers = hasPointers || efns[i].ranges != nil
		}
		if hasPointers {
			f.ranges = func(obj0 reflect.Value, funcs *funcStore, m *memRanges) {
				obj := obj0.(*reflect.StructValue)
				for i, f := range efns {
					if f.ranges != nil {
						f.ranges(obj.Field(i), funcs, m)
					}
				}
			}
			f.copy = func(dst0, obj0 reflect.Value, funcs *funcStore, m *memRanges) {
				dst := dst0.(*reflect.StructValue)
				obj := obj0.(*reflect.StructValue)
				for i, f := range efns {
					f.copy(dst.Field(i), obj.Field(i), funcs, m)
				}
			}
		} else {
			f.ranges = nil
			f.copy = shallowCopy
		}

	default:
		f.ranges = nil
		f.copy = shallowCopy
	}
	return
}

// placeholder function for actual range function
func dummyRanges(_ reflect.Value, _ *funcStore, _ *memRanges) {
	panic("dummyRanges should not have been called")
}

func shallowCopy(dst, obj reflect.Value, _ *funcStore, _ *memRanges) {
	dst.SetValue(obj)
}

func (f *funcStore) get(t reflect.Type) *copyFuncs {
	if fns := f.funcs[t]; fns != nil {
		return fns
	}
	if f.nfuncs != nil {
		return f.nfuncs[t]
	}
	return nil
}

func (f *funcStore) set(t reflect.Type, fns *copyFuncs) {
	if f.nfuncs == nil {
		f.nfuncs = make(map[reflect.Type]*copyFuncs)
	}
	f.nfuncs[t] = fns
}

func makeSlice(r memRange, t0 reflect.Type) (v reflect.Value, allocAddr uintptr) {
	t := t0.(*reflect.SliceType)
	esize := t.Elem().Size()
	n := (r.m1 - r.m0) / esize
	s := reflect.MakeSlice(t, int(n), int(n))
	return s, s.Get() - n*esize
}

func makeZero(_ memRange, t reflect.Type) (v reflect.Value, allocAddr uintptr) {
	v = reflect.MakeZero(t)
	return v, v.Addr()
}

func (l *memRangeList) String() string {
	s := "["
	for ; l != nil; l = l.next {
		s += fmt.Sprintf("(%x-%x: %v), ", l.m0, l.m1, l.t)
	}
	s += "]"
	return s
}

// add tries to add the provided range to the list of known memory allocations.
// If the range is within an existing range, it returns false.
// If the range subsumes existing ranges, they are deleted and replaced
// with the new range.
// mk is a function to be called to allocate space for a copy of the memory,
// which will be called with t and r (to avoid a closure allocation)
//
func (m *memRanges) add(r memRange, t reflect.Type, mk func(r memRange, t reflect.Type) (reflect.Value, uintptr)) (added bool) {
	prev := &m.l
	var s *memRangeList
	for s = *prev; s != nil; s = s.next {
		if r.m1 <= s.m0 {
			// not found: add a new range
			break
		}
		if r.m0 >= s.m0 && r.m1 <= s.m1 {
			// r is within s
			return false
		}
		if r.m0 <= s.m0 {
			// r contains s (and possibly following ranges too),
			// so delete s
			if r.m1 < s.m1 {
				panic("overlapping range")
			}
			*prev = s.next
			continue
		}
		prev = &s.next
	}
	*prev = &memRangeList{r, t, nil, 0, false, mk, s}
	return true
}

// get looks for a memory range that contains m0.
//
func (m *memRanges) get(m0 uintptr) *memRangeList {
	for l := m.l; l != nil; l = l.next {
		if m0 < l.m0 {
			break
		}
		if m0 < l.m1 {
			return l
		}
	}
	return nil
}


func (r memRange) String() string {
	return fmt.Sprintf("[%x %x]", r.m0, r.m1)
}
