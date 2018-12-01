package b

import "github.com/rogpeppe/godef/a"

type S1 struct { //@S1
	F1 int //@mark(S1F1, "F1")
	f2 int
	f3 S2
	S2 //@godef("S2", S2), mark(S1S2, "S2")
}

type S2 struct { //@S2
	F1 string //@mark(S2F1, "F1")
	F2 int    //@mark(S2F2, "F2")
}

func Bar() {
	a.Stuff() //@godef("Stuff", Stuff)
	var x S1  //@godef("S1", S1)
	x.S2      //@godef("S2", S1S2)
	x.F1      //@godef("F1", S1F1)
	x.F2      //@godef("F2", S2F2)
	x.S2.F1   //@godef("F1", S2F1)
}
