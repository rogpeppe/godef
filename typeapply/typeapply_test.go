package typeapply
import (
	"testing"
)

type Target struct {
	i int
	j int
}

type T struct {
	Next *T
	Targ *Target
}

func initTarget(t *Target) {
	t.j = t.i
}

func targ(i int) *Target {
	return &Target{i: i}
}

func checkTarget(t *testing.T, targ *Target) {
	if targ.i != targ.j {
		t.Fatalf("expected %d; got %d", targ.i, targ.j)
	}
}

func TestLinked(t *testing.T) {
	var hd *T
	for i := 0; i < 10; i++ {
		hd = &T{Next: hd, Targ: targ(i)}
	}
	Do(initTarget, hd)
	for ; hd != nil; hd = hd.Next {
		checkTarget(t, hd.Targ)
	}
}

func TestArray(t *testing.T) {
	a := make([]*Target, 10)
	for i := range a {
		a[i] = targ(i)
	}
	Do(initTarget, a)
	for _, targ := range a {
		checkTarget(t, targ)
	}
}

func TestMap(t *testing.T) {
	m := make(map[*T] *T)
	for i := 0; i < 10; i++ {
		k := &T{&T{nil, targ(20+i)}, targ(40+i)}
		v := &T{&T{nil, targ(60+i)}, targ(80+i)}
		m[k] = v
	}
	Do(initTarget, m)
	for k, v := range m {
		checkTarget(t, k.Targ)
		checkTarget(t, k.Next.Targ)
		checkTarget(t, v.Targ)
		checkTarget(t, v.Next.Targ)
	}
}
