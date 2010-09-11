package deepcopy

import (
	"reflect"
	"testing"
)

func TestRangeAmalgamation(t *testing.T) {
	inputs := []memRange{
			memRange{2, 4},
			memRange{6, 8},
			memRange{0, 2},
			memRange{0, 4},
			memRange{10, 20},
			memRange{0, 8},
		}
	outputs := [][]memRange{
		[]memRange{
			memRange{2, 4},
		},
		[]memRange{
			memRange{2,4}, memRange{6, 8},
		},
		[]memRange{
			memRange{0, 2}, memRange{2, 4}, memRange{6, 8},
		},
		[]memRange{
			memRange{0, 4}, memRange{6, 8},
		},
		[]memRange{
			memRange{0, 4}, memRange{6, 8}, memRange{10, 20},
		},
		[]memRange{
			memRange{0, 8}, memRange{10, 20},
		},
	}

	var l memRanges
	i := 0
	for _, r := range inputs {
		l.add(r, nil, nil)
		j := 0
		for rl := l.l; rl != nil; rl = rl.next {
			if rl.m0 != outputs[i][j].m0 || rl.m1 != outputs[i][j].m1 {
				t.Fatal("step %d, want %s, got %s", i, outputs[i], l)
			}
			j++
		}
		i++
	}
}

type copyTest struct {
	v interface{}
	eq func(v1, v2 interface{}) bool
}

func simpleEq(v0, v1 interface{}) bool {
	return reflect.DeepEqual(v0, v1)
}

var slice = []int{0,1,2,3,4,5,6}

type uintPtrGetter interface {
	Get() uintptr
}

func eqPtr(x, y interface{}) bool {
	v := reflect.NewValue(x).(uintPtrGetter)
	w := reflect.NewValue(y).(uintPtrGetter)

	return v.Get() == w.Get()
}
type T1 struct {
	A *int
	B, C []int
}
type T2 struct {
	A int
	B *int
	C *T2
}
type T3 struct {
	A, B map[string] int
}
type T4 struct {
	A *[]int
	B []int
	C [4]int
}

type Tree struct {
	L, R *Tree
}


func TestDeepCopy(t *testing.T) {
	root := &Tree{}
	*root = Tree{
		&Tree{
			root,
			root,
		},
		&Tree{
			root,
			root,
		},
	}
	t2 := &T2{99, nil, nil}
	t4 := &T4{nil, nil, [...]int{0, 1, 2, 3}}
	t4.A = &t4.B
	t4.B = t4.C[1:]
	m := map[string]int{"one": 1, "two": 2}
	var tests = []copyTest{
		copyTest{
			5,
			simpleEq,
		},
		copyTest{
			[]int{3, 5, 7},
			simpleEq,
		},
		copyTest{
			T1{
				&slice[1],
				slice[2 : 5],
				slice[0 : 4],
			},
			func(x, y interface{}) bool {
				y1 := y.(T1)
				return simpleEq(x, y) && eqPtr(y1.B, y1.C) && y1.A == &y1.C[1]
			},
		},
		copyTest{
			root,
			func(x, y interface{}) bool {
				y1 := y.(*Tree)
				return simpleEq(x, y) &&
					y1.L.L == y1 &&
					y1.L.R == y1 &&
					y1.R.L == y1 &&
					y1.R.R == y1 &&
					y1 != root
			},
		},
		copyTest{
			&T2{88, &t2.A, t2},
			func(x, y interface{}) bool {
				y1 := y.(*T2)
				return simpleEq(x, y) &&
					y1.B == &y1.C.A &&
					y1.C.A == 99
			},
		},
		copyTest{
			T3{m, m},
			func(x, y interface{}) bool {
				y1 := y.(T3)
				return simpleEq(x, y) &&
					y1.A == y1.B
			},
		},
		copyTest{
			map[*T2]map[string]int {
				t2: m,
			},
			func(x, y interface{}) bool {
				y1 := y.(map[*T2]map[string]int)
				if !simpleEq(x, y) {
					return false
				}
				for _, v := range y1 {
					if v == m {
						return false
					}
					if v["one"] != 1 {
						return false
					}
				}
				return true
			},
		},
		copyTest{
			t4,
			func(x, y interface{}) bool {
				y1 := y.(*T4)
				return simpleEq(x, y) &&
					x != y &&
					y1.A == &y1.B &&
					&y1.B[0] == &y1.C[1]
			},
		},
	}
	for i, test := range tests {
		v1 := DeepCopy(test.v)
		if !test.eq(test.v, v1) {
			t.Errorf("failure at %d on %v -> %v\n", i, test.v, v1)
		}
	}
}
