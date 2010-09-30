package extend

import (
	"testing"
	"runtime"
)

const N = 200

func TestInt(t *testing.T) {
	var a []int
	push := Pusher(&a)
	for i := 0; i < N; i++ {
		push(i)
	}
	if len(a) != N {
		t.Fatalf("array size %d; expected %d\n", len(a), N)
	}
	for i, x := range a {
		if i != x {
			t.Fatalf("array element %d; expected %d\n", x, i)
		}
	}
}

func TestLarge(t *testing.T) {
	type Large [5]int
	var a []Large
	push := Pusher(&a)
	for i := 0; i < N; i++ {
		push(Large{i})
	}
	if len(a) != N {
		t.Fatalf("array size %d; expected %d\n", len(a), N)
	}
	for i, x := range a {
		if i != x[0] {
			t.Fatalf("array element %d; expected %d\n", x[0], i)
		}
	}
}

type X int
func (_ X) Foo() { }

func TestInterface(t *testing.T) {
	type T interface {
		Foo()
	}
	var a []T
	push := Pusher(&a)
	for i := 0; i < N; i++ {
		push(X(i))
	}
	if len(a) != N {
		t.Fatalf("array size %d; expected %d\n", len(a), N)
	}
	for i, x := range a {
		if i != int(x.(X)) {
			t.Fatalf("array element %d; expected %d\n", x, i)
		}
	}
}

func TestTypeChecking(t *testing.T) {
	var a []int
	push := Pusher(&a)
	defer func(){
		if v, ok := recover().(string); !ok {
			t.Fatalf("expected panic; got %v\n", v)
		}
	}()
	push("hello")
}

// this benchmark mirrors BenchmarkVectorNums in container/vector
// for comparison purposes.
func BenchmarkPush(b *testing.B) {
	c := int(0)
	var a []int
	b.StopTimer()
	runtime.GC()
	b.StartTimer()
	push := Pusher(&a)
	for i := 0; i < b.N; i++ {
		push(c)
	}
}
