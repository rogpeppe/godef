package b

import "github.com/rogpeppe/godef/a"

func Bar() {
	a.Stuff() //@godef("Stuff", Stuff)
}
