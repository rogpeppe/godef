package extend
import (
	"reflect"
	"unsafe"
)

const initialSize = 4

// This type mirrors the actual runtime interface type.
type interfaceHeader struct {
	t uintptr
	data uintptr
}

// Pusher must be passed a pointer to a slice. It returns
// a function that pushes a new value onto the end of the
// slice, reallocating the slice if necessary.
func Pusher(ap interface{}) func(interface{}) {
	v := reflect.NewValue(ap).(*reflect.PtrValue).Elem().(*reflect.SliceValue)
	h := (*reflect.SliceHeader)(unsafe.Pointer(v.Addr()))
	t := v.Type().(*reflect.SliceType)
	elemType := t.Elem()

	if _, ok := elemType.(*reflect.InterfaceType); ok {
		// If the array's element type is an interface,
		// then we must go the slow route, because
		// the type being pushed will not be identical
		// the type in the array, so we rely on reflect
		// to do the value->interface conversion.
		return func(x interface{}) {
			len, cap := h.Len, h.Cap
			if len < cap {
				h.Len++
			}else{
				if cap == 0 {
					cap = initialSize
				}else{
					cap *= 2
				}
				b := reflect.MakeSlice(t, len+1, cap)
				reflect.ArrayCopy(b, v)
				v.SetValue(b)
			}
			v.Elem(len).SetValue(reflect.NewValue(x))
		}
	}
	// Nasty unsafe hackery:
	//
	// We copy each element to xcopy, so that we can take
	// the address of it without triggering the allocator.
	// We set up e1 as a []byte alias to the data,
	// and do the actual copy by setting up e0 as a []byte
	// alias to the array element and invoking copy().
	// TODO: enable this code for type with size > sizeof(uintptr).
	esize := elemType.Size()
	valueInsideInterface := esize <= uintptr(unsafe.Sizeof(uintptr(0)))

	var e0 []byte
	var e1 []byte
	he0 := (*reflect.SliceHeader)(unsafe.Pointer(&e0))
	he1 := (*reflect.SliceHeader)(unsafe.Pointer(&e1))

	var xcopy interface{}
	ih := (*interfaceHeader)(unsafe.Pointer(&xcopy))
	if valueInsideInterface {
		he1.Data = uintptr(unsafe.Pointer(&ih.data))
		he1.Len = int(esize)
		he1.Cap = int(esize)
	}

	return func(x interface{}) {
		if reflect.Typeof(x) != elemType {
			panic("wrong type pushed")
		}
		len, cap := h.Len, h.Cap
		if len < cap {
			h.Len++
		}else{
			if cap == 0 {
				cap = initialSize
			}else{
				cap *= 2
			}
			b := reflect.MakeSlice(t, len+1, cap)
			reflect.ArrayCopy(b, v)
			v.SetValue(b)
		}
		xcopy = x
		he0.Data = h.Data + esize * uintptr(len)
		he0.Len = int(esize)
		he0.Cap = int(esize)
		if valueInsideInterface {
			copy(e0, e1)
		}else{
			he1.Data = ih.data
			he1.Len = int(esize)
			he1.Cap = int(esize)
			copy(e0, e1)
			e1 = nil
		}
		e0 = nil
	}
}
