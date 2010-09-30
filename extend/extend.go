package extend
import (
	"reflect"
	"unsafe"
)

const initialSize = 4

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
	esize := t.Elem().Size()
	_, isInterface := t.Elem().(*reflect.InterfaceType)
	unsafeCopy := !isInterface && esize <= uintptr(unsafe.Sizeof(uintptr(0)))
	if !unsafeCopy {
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
	// We know that the size of the type fits in a pointer,
	// so the value is held directly inside the interface value.
	// We copy each element to icopy, so that we can take
	// the address of it without triggering the allocator.
	// We set up e1 as a []byte alias to the data,
	// and do the actual copy by setting up e0 as a []byte
	// alias to the array element and invoking copy().
	// TODO: enable this code for type with size > sizeof(uintptr).
	var e0 []byte
	var e1 []byte
	he0 := (*reflect.SliceHeader)(unsafe.Pointer(&e0))
	he1 := (*reflect.SliceHeader)(unsafe.Pointer(&e1))
	var icopy interface{}
	he1.Data = uintptr(unsafe.Pointer(&icopy)) + uintptr(unsafe.Offsetof(interfaceHeader{}.data))
	he1.Len = int(esize)
	he1.Cap = int(esize)

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
		icopy = x
		he0.Data = h.Data + esize * uintptr(len)
		he0.Len = int(esize)
		he0.Cap = int(esize)
		copy(e0, e1)
		e0 = nil
	}
}
