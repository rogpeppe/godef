package b

// This is the in-editor version of the file.
// The on-disk version is in c.go.saved.

var _ = S1{ //@godef("S1", S1)
	F1: 99, //@_godef("F1", S1F1) //disabled, godef cannot resolve struct literal fields yet
}
