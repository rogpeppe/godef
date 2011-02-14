package typeapply
import (
	"reflect"
	"sync"
)

type traverserFunc func(*reflect.FuncValue, reflect.Value)

var (
	typeMutex sync.Mutex

	// map from function's first argument type to a map
	// from every type contained in that type to
	// a function that can traverse that type.
	tmap = make(map[reflect.Type] map[reflect.Type] *typeInfo)
)

type knowledge byte
const (
	dontKnow = knowledge(iota)
	no
	yes
)

type typeInfo struct {
	canReach knowledge
	visiting bool
	trav traverserFunc
}

// Do calls the function f, which must
// be of the form func(T) for some type T, on
// each publicly accessible member of x
// and recursively on members of those.
// It will fail with infinite recursion if the x
// is cyclic.
func Do(f interface{}, x interface{}){
	fv := reflect.NewValue(f).(*reflect.FuncValue)
	ft := fv.Type().(*reflect.FuncType)
	if ft.NumIn() != 1 {
		panic("function should take exactly one argument only")
	}
	if ft.NumOut() != 0 {
		panic("function should have no return values")
	}
	xv := reflect.NewValue(x)
	xt := xv.Type()

	// t is the type that we're looking for.
	t := ft.In(0)

	typeMutex.Lock()
	m := tmap[t]
	if m == nil {
		m = make(map[reflect.Type] *typeInfo)
		tmap[t] = m
	}
	trav := getTraverserFunc(m, t, xt)
	typeMutex.Unlock()
	if trav != nil {
		trav(fv, xv)
	}
}

func dummyTraverser(fv *reflect.FuncValue, xv reflect.Value){
	panic("dummyTraverser called")
}

func getTraverserFunc(m map[reflect.Type] *typeInfo, t, xt reflect.Type) traverserFunc {
	needFunc := canReach(m, t, xt)
	info := m[xt]
	if info.trav != nil || needFunc == no {
		return info.trav
	}
	// If the answer is "don't know", then it can only be
	// be because we have a recursive type with no instances of t.
	if needFunc == dontKnow {
		info.canReach = no
		return nil
	}
	// If we've actually found an instance of the type, then
	// the traverserFunc calls the function on it.
	if xt == t {
		info.trav = func(fv *reflect.FuncValue, xv reflect.Value){
			fv.Call([]reflect.Value{xv})
		}
		return info.trav
	}
	// prime the typeInfo with a dummy function, so that
	// getTraverserFunc returns non-nil when called recursively
	// on types that can reach t. It will be reset before the
	// end of the function, and the only way we can recurse
	// is through a StructValue, which indirects through the typeInfo,
	// so will use the eventual value.
	info.trav = dummyTraverser
	switch xt := xt.(type) {
	case *reflect.PtrType:
		elemFunc := getTraverserFunc(m, t, xt.Elem())
		info.trav = func(fv *reflect.FuncValue, xv reflect.Value){
			if y := xv.(*reflect.PtrValue); !y.IsNil() {
				elemFunc(fv, y.Elem())
			}
		}

	case *reflect.StructType:
		n := xt.NumField()
		type fieldFunc struct {
			i int
			info *typeInfo
		}
		fields := make([]fieldFunc, n)
		j := 0
		for i := 0; i < n; i++ {
			ft := xt.Field(i).Type
			if getTraverserFunc(m, t, ft) != nil {
				fields[j] = fieldFunc{i, m[ft]}
				j++
			}
		}
		fields = fields[0:j]
		info.trav = func(fv *reflect.FuncValue, xv reflect.Value){
			y := xv.(*reflect.StructValue)
			for _, f := range fields {
				// indirect through info so that recursive types work ok.
				f.info.trav(fv, y.Field(f.i))
			}
		}

	case *reflect.MapType:
		travKey := getTraverserFunc(m, t, xt.Key())
		travElem := getTraverserFunc(m, t, xt.Elem())
		info.trav = func(fv *reflect.FuncValue, xv reflect.Value){
			y := xv.(*reflect.MapValue)
			for _, key := range y.Keys() {
				if travKey != nil {
					travKey(fv, key)
				}
				if travElem != nil {
					travElem(fv, y.Elem(key))
				}
			}
		}

	case reflect.ArrayOrSliceType:
		trav := getTraverserFunc(m, t, xt.Elem())
		info.trav = func(fv *reflect.FuncValue, xv reflect.Value){
			y := xv.(reflect.ArrayOrSliceValue)
			n := y.Len()
			for i := 0; i < n; i++ {
				trav(fv, y.Elem(i))
			}
		}

	case *reflect.InterfaceType:
		info.trav = func(fv *reflect.FuncValue, xv reflect.Value){
			y := xv.(*reflect.InterfaceValue)
			if y.IsNil() {
				return
			}
			elem := y.Elem()
			typeMutex.Lock()
			trav := getTraverserFunc(m, t, elem.Type())
			typeMutex.Unlock()
			if trav != nil {
				trav(fv, elem)
			}
		}
	default:
		panic("unexpected type reached: "+xt.String())
	}
	return info.trav
}

// canReach returns whether t can be reached from xt.
func canReach(m map[reflect.Type] *typeInfo, t, xt reflect.Type) knowledge {
	if info := m[xt]; info != nil {
		if info.visiting || info.canReach != dontKnow {
			return info.canReach
		}
	}
	info := new(typeInfo)
	m[xt] = info
	if t == xt {
		info.canReach = yes
		return yes
	}
	info.visiting = true
	switch xt := xt.(type) {
	case *reflect.PtrType:
		info.canReach = canReach(m, t, xt.Elem())

	case *reflect.StructType:
		for i := 0; i < xt.NumField(); i++ {
			f := xt.Field(i)
			// ignore unexported members.
			if f.PkgPath == "" {
				info.canReach = or(canReach(m, t,  f.Type), info.canReach)
			}
		}

	case *reflect.MapType:
		info.canReach = canReach(m, t, xt.Key())
		info.canReach = or(canReach(m, t, xt.Elem()), info.canReach)

	case reflect.ArrayOrSliceType:
		info.canReach = canReach(m, t, xt.Elem())

	case *reflect.InterfaceType:
		info.canReach = yes
	}
	info.visiting = false
	return info.canReach
}

func or(a, b knowledge) knowledge {
	switch {
	case a == yes || b == yes:
		return yes
	case a == dontKnow || b == dontKnow:
		return dontKnow
	}
	return no
}

// Is this an exported - upper case - name?
//func isExported(name string) bool {
//	rune, _ := utf8.DecodeRuneInString(name)
//	return unicode.IsUpper(rune)
//}
