package typeapply

import (
	"testing"
)

type Targ struct {
}

type List struct {
	Next *List
	Targ *Targ
}

type Other struct {
	Targ *Targ
}

type Big struct {
	A *Targ
	B [2]*Targ
	C []*Targ
	D map[int]*Targ
	E map[*Targ]int
	F map[*Targ]*Targ
	G ***Targ
	H interface{}
	I *List
	J List
	// no instances:
	K chan *Targ
	L func(*Targ) *Targ
}

func newBig() (big *Big, n int) {
	T := func() *Targ {
		n++
		return &Targ{}
	}
	pppt := func() ***Targ {
		pt := new(*Targ)
		*pt = T()
		ppt := new(**Targ)
		*ppt = pt
		return ppt
	}
	big = &Big{
		A: T(),
		B: [2]*Targ{T(), T()},
		C: []*Targ{T(), T()},
		D: map[int]*Targ{1: T(), 2: T()},
		E: map[*Targ]int{T(): 1, T(): 2},
		F: map[*Targ]*Targ{T(): T(), T(): T()},
		G: pppt(),
		H: Other{T()},
		I: &List{&List{nil, T()}, T()},
		J: List{&List{nil, T()}, T()},
		K: make(chan *Targ),
		L: func(*Targ) *Targ { return nil },
	}
	return
}

func TestBig(t *testing.T) {
	b, n := newBig()
	i := 0
	m := make(map[*Targ]bool)
	Do(func(targ *Targ) {
		i++
		if m[targ] {
			t.Error("target reached twice")
		}
	}, b)
	if i != n {
		t.Errorf("wrong instance count; expected %d; got %d", n, i)
	}
}
