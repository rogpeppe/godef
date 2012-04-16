package typeapply

import (
	"reflect"
	"sync"
)

var (
	typeMutex sync.Mutex

	// tmap stores a map for every target type (the
	// argument type of the function passed to Do).
	// For each target type, we use this to cache
	// information about how to traverse values
	// of all types that have been passed to Do
	// (and any types that they refer to).
	tmap = make(map[reflect.Type]map[reflect.Type]*typeInfo)
)

// A traverserFunc implements Do for a particular
// value type X and target type T. It assumes that x is of
// type X and f is of type func(T).
type traverserFunc func(f reflect.Value, x reflect.Value)

type knowledge byte

const (
	dontKnow = knowledge(iota) // don't know yet
	no                         // definite no.
	yes                        // definite yes.
)

// typeInfo holds information on a value type with respect to a particular
// target type.
type typeInfo struct {
	// Can this type contain instances of the target type?
	// This can be dontKnow during execution of the canReach
	// function
	canReach knowledge

	// Is this type being currently visited?
	visiting bool

	// A function that implements Do on this type.
	trav traverserFunc
}

// Do calls the function f, which must be of the form func(T) for some
// type T, on each publicly accessible member of the value x and recursively on
// members of those. It will fail with infinite recursion if x is
// cyclic. If a member of x is reachable in more than one way,
// then f will be called for each way.
func Do(f interface{}, x interface{}) {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if ft.NumIn() != 1 {
		panic("function should take exactly one argument only")
	}
	if ft.NumOut() != 0 {
		panic("function should have no return values")
	}
	xv := reflect.ValueOf(x)
	xt := xv.Type()

	// t is the type that we're looking for.
	t := ft.In(0)

	typeMutex.Lock()
	m := tmap[t]
	if m == nil {
		m = make(map[reflect.Type]*typeInfo)
		tmap[t] = m
	}
	trav := getTraverserFunc(m, t, xt)
	typeMutex.Unlock()
	if trav != nil {
		trav(fv, xv)
	}
}

// getTraverserFunc returns the traverserFunc for a given target type t
// and and value type xt. It returns nil if xt cannot contain any
// instance of t.
func getTraverserFunc(m map[reflect.Type]*typeInfo, t, xt reflect.Type) traverserFunc {
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
		info.trav = func(fv reflect.Value, xv reflect.Value) {
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
	switch xt.Kind() {
	case reflect.Ptr:
		elemFunc := getTraverserFunc(m, t, xt.Elem())
		info.trav = func(fv reflect.Value, xv reflect.Value) {
			if y := xv; !y.IsNil() {
				elemFunc(fv, y.Elem())
			}
		}

	case reflect.Struct:
		n := xt.NumField()
		type fieldFunc struct {
			i    int
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
		info.trav = func(fv reflect.Value, xv reflect.Value) {
			y := xv
			for _, f := range fields {
				// indirect through info so that recursive types work ok.
				f.info.trav(fv, y.Field(f.i))
			}
		}

	case reflect.Map:
		travKey := getTraverserFunc(m, t, xt.Key())
		travElem := getTraverserFunc(m, t, xt.Elem())
		info.trav = func(fv reflect.Value, xv reflect.Value) {
			y := xv
			for _, key := range y.MapKeys() {
				if travKey != nil {
					travKey(fv, key)
				}
				if travElem != nil {
					travElem(fv, y.MapIndex(key))
				}
			}
		}

	case reflect.Array, reflect.Slice:
		trav := getTraverserFunc(m, t, xt.Elem())
		info.trav = func(fv reflect.Value, xv reflect.Value) {
			y := xv
			n := y.Len()
			for i := 0; i < n; i++ {
				trav(fv, y.Index(i))
			}
		}

	case reflect.Interface:
		info.trav = func(fv reflect.Value, xv reflect.Value) {
			y := xv
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
		panic("unexpected type reached: " + xt.String())
	}
	return info.trav
}

// canReach returns whether t can be reached from xt.
func canReach(m map[reflect.Type]*typeInfo, t, xt reflect.Type) knowledge {
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
	switch xt.Kind() {
	case reflect.Ptr:
		info.canReach = canReach(m, t, xt.Elem())

	case reflect.Struct:
		for i := 0; i < xt.NumField(); i++ {
			f := xt.Field(i)
			// ignore unexported members.
			if f.PkgPath == "" {
				info.canReach = or(canReach(m, t, f.Type), info.canReach)
			}
		}

	case reflect.Map:
		info.canReach = canReach(m, t, xt.Key())
		info.canReach = or(canReach(m, t, xt.Elem()), info.canReach)

	case reflect.Array:
		info.canReach = canReach(m, t, xt.Elem())

	case reflect.Slice:
		info.canReach = canReach(m, t, xt.Elem())

	case reflect.Interface:
		info.canReach = yes
	}
	info.visiting = false
	return info.canReach
}

func dummyTraverser(fv reflect.Value, xv reflect.Value) {
	panic("dummyTraverser called")
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
