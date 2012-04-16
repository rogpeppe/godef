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
			memRange{2, 4}, memRange{6, 8},
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
	v  interface{}
	eq func(v1, v2 interface{}) bool
}

func simpleEq(v0, v1 interface{}) bool {
	return reflect.DeepEqual(v0, v1)
}

var slice = []int{0, 1, 2, 3, 4, 5, 6}

type uintPtrGetter interface {
	Get() uintptr
}

func eqPtr(x, y interface{}) bool {
	v := reflect.ValueOf(x).(uintPtrGetter)
	w := reflect.ValueOf(y).(uintPtrGetter)

	return v.Get() == w.Get()
}

type T1 struct {
	A    *int
	B, C []int
}
type T2 struct {
	A int
	B *int
	C *T2
}
type T3 struct {
	A, B map[string]int
}
type T4 struct {
	A *[]int
	B []int
	C [4]int
}
type T5 struct {
	A interface{}
	B interface{}
}

type Tree struct {
	L, R *Tree
}

var root *Tree
var t1 T1
var t2 *T2
var t3 T3
var t4 *T4
var t5 *T5
var m = map[string]int{"one": 1, "two": 2}

func init() {
	root = &Tree{}
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
	t1 = T1{
		&slice[1],
		slice[2:5],
		slice[0:4],
	}
	t2 = &T2{99, nil, nil}
	t2 = &T2{88, &t2.A, t2}
	t3 = T3{m, m}
	t4 = &T4{nil, nil, [...]int{0, 1, 2, 3}}
	t4.A = &t4.B
	t4.B = t4.C[1:]
	t5 = &T5{}
	t5.A = t2
	t5.B = t5
}

func TestDeepCopy(t *testing.T) {
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
			t1,
			func(x, y interface{}) bool {
				y1 := y.(T1)
				return eqPtr(y1.B, y1.C) && y1.A == &y1.C[1]
			},
		},
		copyTest{
			root,
			func(x, y interface{}) bool {
				y1 := y.(*Tree)
				return y1.L.L == y1 &&
					y1.L.R == y1 &&
					y1.R.L == y1 &&
					y1.R.R == y1 &&
					y1 != root
			},
		},
		copyTest{
			t2,
			func(x, y interface{}) bool {
				y1 := y.(*T2)
				return y1.B == &y1.C.A &&
					y1.C.A == 99
			},
		},
		copyTest{
			t3,
			func(x, y interface{}) bool {
				y1 := y.(T3)
				return y1.A == y1.B
			},
		},
		copyTest{
			map[*T2]map[string]int{
				t2: m,
			},
			func(x, y interface{}) bool {
				y1 := y.(map[*T2]map[string]int)
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
				return x != y &&
					y1.A == &y1.B &&
					&y1.B[0] == &y1.C[1]
			},
		},
		copyTest{
			t5,
			func(x, y interface{}) bool {
				y1 := y.(*T5)
				return y1.A.(*T2).A == 88 &&
					y1.B.(*T5) == y1
			},
		},
	}
	for i, test := range tests {
		v1 := Copy(test.v)
		if !simpleEq(test.v, v1) {
			t.Errorf("simpleEq failure at %d on %v -> %v\n", i, test.v, v1)
		}
		if !test.eq(test.v, v1) {
			t.Errorf("failure at %d on %v -> %v\n", i, test.v, v1)
		}
	}
}

func BenchmarkCopyT1(b *testing.B) {
	for i := b.N; i >= 0; i-- {
		Copy(t1)
	}
}

func BenchmarkCopyT2(b *testing.B) {
	for i := b.N; i >= 0; i-- {
		Copy(t2)
	}
}

func BenchmarkCopyT3(b *testing.B) {
	for i := b.N; i >= 0; i-- {
		Copy(t3)
	}
}

func BenchmarkCopyT4(b *testing.B) {
	for i := b.N; i >= 0; i-- {
		Copy(t4)
	}
}

func BenchmarkCopyT5(b *testing.B) {
	for i := b.N; i >= 0; i-- {
		Copy(t5)
	}
}
