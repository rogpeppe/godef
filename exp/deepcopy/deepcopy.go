// The deepcopy package implements deep copying of arbitrary
// data structures, making sure that self references and shared pointers
// are preserved.
package deepcopy

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"
)

// basic copy algorithm:
// 1) recursively scan object, building up a list of all allocated
// memory ranges pointed to by the object.
// 2) recursively copy object, making sure that references
// within the data structure point to the appropriate
// places in the newly allocated data structure.
//
// TO DO: leave any pointers to structs with unexported fields
// untouched, rather than failing entirely.

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
type copyInfo struct {
	hasPointers bool
	canCopy     bool
	ranges      func(obj reflect.Value, f *infoStore, m *memRanges)
	copy        func(dst, obj reflect.Value, f *infoStore, m *memRanges)
}

var infoMap = make(map[reflect.Type]*copyInfo)
var lock sync.Mutex

// infoStore is essentially just a single map, but
// is stored as two so that new types can be added
// to the map concurrently without placing a lock around every
// map access. This works because the same functions
// will be generated for any given type.
//
type infoStore struct {
	info, newInfo map[reflect.Type]*copyInfo
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
	v := reflect.ValueOf(obj)

	lock.Lock()
	store := infoStore{infoMap, nil}
	f := deepCopyInfo(&store, v.Type())
	lock.Unlock()

	f.ranges(v, &store, &m)
	dst := reflect.Zero(v.Type())
	f.copy(dst, v, &store, &m)

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
	if store.newInfo != nil {
		lock.Lock()
		for k, v := range store.info {
			store.newInfo[k] = v
		}
		infoMap = store.newInfo
		lock.Unlock()
	}
	return dst.Interface()
}

// deepCopyInfo returns the copyInfo for the given type t,
// To save recursively inspecting its type each time an object
// is copied, we store (in store) functions tailored to each type that
// know how to calculate ranges and copy objects of that type.
//
func deepCopyInfo(store *infoStore, t reflect.Type) (f *copyInfo) {
	if f = store.get(t); f != nil {
		return
	}
	// must add to store before switch so that recursive types work.
	// hasPointers is must be true so that the recursion will terminate.
	f = &copyInfo{true, true, nil, nil}
	store.set(t, f)
	switch t.Kind() {
	case reflect.Interface:
		f.ranges = func(obj0 reflect.Value, store *infoStore, m *memRanges) {
			obj := obj0
			e := obj.Elem()
			deepCopyInfo(store, e.Type()).ranges(e, store, m)
		}
		f.copy = func(dst, obj0 reflect.Value, store *infoStore, m *memRanges) {
			e := obj0.Elem()
			einfo := deepCopyInfo(store, e.Type())

			// we cannot just pass dst to einfo.copy() because
			// various types (e.g. arrays and structs) expect an 
			// actual instance of the type to modify
			v := reflect.Zero(e.Type())
			einfo.copy(v, e, store, m)
			dst.Set(v)
		}

	case reflect.Map:
		et := t.Elem()
		einfo := deepCopyInfo(store, et)
		f.ranges = func(obj0 reflect.Value, store *infoStore, m *memRanges) {
			obj := obj0
			m0 := obj.Pointer()
			if m.add(memRange{m0, m0 + 1}, t, nil) {
				if einfo.hasPointers {
					for _, k := range obj.MapKeys() {
						einfo.ranges(obj.MapIndex(k), store, m)
					}
				}
			}
		}
		f.copy = func(dst, obj0 reflect.Value, store *infoStore, m *memRanges) {
			obj := obj0
			m0 := obj.Pointer()
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
				dst.Set(e.v)
				if einfo.hasPointers {
					kv := reflect.Zero(t.Elem())
					for _, k := range obj.MapKeys() {
						einfo.copy(kv, obj.MapIndex(k), store, m)
						v.SetMapIndex(k, kv)
					}
				} else {
					for _, k := range obj.MapKeys() {
						v.SetMapIndex(k, obj.MapIndex(k))
					}
				}
			} else {
				dst.Set(e.v)
			}
		}

	case reflect.Ptr:
		et := t.Elem()
		einfo := deepCopyInfo(store, et)
		esize := et.Size()
		f.ranges = func(obj0 reflect.Value, store *infoStore, m *memRanges) {
			obj := obj0
			if !obj.IsNil() {
				m0 := obj.Pointer()
				if m.add(memRange{m0, m0 + esize}, et, makeZero) {
					if einfo.hasPointers {
						einfo.ranges(obj.Elem(), store, m)
					}
				}
			}
		}
		f.copy = func(dst0, obj0 reflect.Value, store *infoStore, m *memRanges) {
			obj := obj0
			if obj.IsNil() {
				return
			}
			m0 := obj.Pointer()
			m1 := m0 + esize
			e := m.get(m0)
			if e == nil {
				panic("range not found")
			}
			if !e.v.IsValid() {
				e.v, e.allocAddr = e.make(e.memRange, e.t)
			}
			ptr := m0 - e.m0 + e.allocAddr
			v := reflect.ValueOf(unsafe.Unreflect(t, unsafe.Pointer(&ptr)))
			// make a copy only if we're pointing to the entire
			// allocated object; otherwise the copy will be made
			// later when we get to the pointer that does point
			// to it.
			if m0 == e.m0 && m1 == e.m1 && !e.copied {
				e.copied = true
				einfo.copy(v.Elem(), obj.Elem(), store, m)
			}
			dst0.Set(v)
		}

	case reflect.Array:
		einfo := deepCopyInfo(store, t.Elem())
		n := t.Len()
		if einfo.hasPointers {
			f.ranges = func(obj0 reflect.Value, store *infoStore, m *memRanges) {
				obj := obj0
				for i := 0; i < n; i++ {
					einfo.ranges(obj.Index(i), store, m)
				}
			}
			f.copy = func(dst0, obj0 reflect.Value, store *infoStore, m *memRanges) {
				dst := dst0
				obj := obj0
				for i := 0; i < n; i++ {
					einfo.copy(dst.Index(i), obj.Index(i), store, m)
				}
			}
		} else {
			f.ranges = noRanges
			f.copy = func(dst0, obj0 reflect.Value, store *infoStore, m *memRanges) {
				dst := dst0
				obj := obj0
				reflect.ArrayCopy(dst, obj)
			}
		}

	case reflect.Slice:
		et := t.Elem()
		esize := et.Size()
		einfo := deepCopyInfo(store, et)
		f.ranges = func(obj0 reflect.Value, store *infoStore, m *memRanges) {
			obj := obj0
			if obj.IsNil() {
				return
			}
			n := obj.Cap()
			m1 := obj.Pointer()
			m0 := m1 - uintptr(n)*esize
			if m.add(memRange{m0, m1}, obj.Type(), makeSlice) {
				if einfo.hasPointers {
					obj = obj.Slice(0, n)
					for i := 0; i < n; i++ {
						einfo.ranges(obj.Index(i), store, m)
					}
				}
			}
		}
		f.copy = func(dst0, obj0 reflect.Value, store *infoStore, m *memRanges) {
			obj := obj0
			if obj.IsNil() {
				return
			}
			dst := dst0
			cap := obj.Cap()
			m1 := obj.Pointer()
			m0 := m1 - uintptr(obj.Cap())*esize
			e := m.get(m0)
			if e == nil {
				panic("slice range not found")
			}
			// allocate whole object, even if obj represents just a part of it
			if !e.v.IsValid() {
				e.v, e.allocAddr = e.make(e.memRange, e.t)
			}
			// we must fabricate the slice, as we may be pointing
			// to a slice of an in-struct array, rather than another slice.
			h := reflect.SliceHeader{m0 - e.m0 + e.allocAddr, obj.Len(), obj.Cap()}
			dst.Set(reflect.ValueOf(unsafe.Unreflect(t, unsafe.Pointer(&h))))
			if e.m0 == m0 && e.m1 == m1 && !e.copied {
				e.copied = true
				obj := obj.Slice(0, cap)
				dst := dst.Slice(0, cap)
				if einfo.hasPointers {
					for i := 0; i < cap; i++ {
						einfo.copy(dst.Index(i), obj.Index(i), store, m)
					}
				} else {
					reflect.ArrayCopy(dst, obj)
				}
			}
		}

	case reflect.Struct:
		einfo := make([]*copyInfo, t.NumField())
		hasPointers := false
		canCopy := true
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			typeInfo := deepCopyInfo(store, field.Type)
			einfo[i] = typeInfo
			hasPointers = hasPointers || typeInfo.hasPointers
			canCopy = canCopy && typeInfo.canCopy && field.PkgPath == ""
		}
		f.hasPointers = hasPointers
		f.canCopy = canCopy
		if hasPointers && canCopy {
			f.ranges = func(obj0 reflect.Value, store *infoStore, m *memRanges) {
				obj := obj0
				for i, f := range einfo {
					if f.hasPointers {
						f.ranges(obj.Field(i), store, m)
					}
				}
			}
			f.copy = func(dst0, obj0 reflect.Value, store *infoStore, m *memRanges) {
				dst := dst0
				obj := obj0
				for i, f := range einfo {
					f.copy(dst.Field(i), obj.Field(i), store, m)
				}
			}
		} else {
			f.ranges = noRanges
			f.copy = shallowCopy
		}

	default:
		f.ranges = noRanges
		f.copy = shallowCopy
	}
	return
}

func noRanges(_ reflect.Value, _ *infoStore, _ *memRanges) {
}

func shallowCopy(dst, obj reflect.Value, _ *infoStore, _ *memRanges) {
	dst.Set(obj)
}

func (f *infoStore) get(t reflect.Type) *copyInfo {
	if i := f.info[t]; i != nil {
		return i
	}
	if f.newInfo != nil {
		return f.newInfo[t]
	}
	return nil
}

func (f *infoStore) set(t reflect.Type, fns *copyInfo) {
	if f.newInfo == nil {
		f.newInfo = make(map[reflect.Type]*copyInfo)
	}
	f.newInfo[t] = fns
}

func makeSlice(r memRange, t0 reflect.Type) (v reflect.Value, allocAddr uintptr) {
	t := t0
	esize := t.Elem().Size()
	n := (r.m1 - r.m0) / esize
	s := reflect.MakeSlice(t, int(n), int(n))
	return s, s.Pointer() - n*esize
}

func makeZero(_ memRange, t reflect.Type) (v reflect.Value, allocAddr uintptr) {
	v = reflect.Zero(t)
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
