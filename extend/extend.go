package extend
import (
	"reflect"
)

const initialSize = 4

// Pusher must be passed a pointer to a slice. It returns
// a function that pushes a new value onto the end of the
// slice, reallocating the slice if necessary.
func Pusher(ap interface{}) func(interface{}) {
	v := reflect.NewValue(ap).(*reflect.PtrValue).Elem().(*reflect.SliceValue)
	t := v.Type().(*reflect.SliceType)
	return func(x interface{}) {
		len, cap := v.Len(), v.Cap()
		if len < cap {
			v.SetLen(len+1)
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
