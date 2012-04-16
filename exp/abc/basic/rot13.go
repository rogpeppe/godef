package basic

import (
	"code.google.com/p/rog-go/exp/abc"
)

func init() {
	abc.Register("rot13", map[string]abc.Socket{
		"out": abc.Socket{FdT, abc.Male},
		"1":   abc.Socket{FdT, abc.Female},
	}, makeRot13)
}
func makeRot13(_ *abc.Status, args map[string]interface{}) abc.Widget {
	in := args["1"].(Fd)
	out := NewFd()
	args["out"].(chan interface{}) <- out
	go rot13proc(in, out)
	return nil
}

func rot13proc(in, out Fd) {
	r := in.GetReader()
	defer close(r)

	w := out.GetWriter(nil)
	defer close(w)

	buf := make([]byte, 8192)
	for {
		n, _ := r.Read(buf)
		if n <= 0 {
			break // propagate error?
		}
		rot13(buf[0:n])
		if m, _ := w.Write(buf[0:n]); m != n {
			break
		}
	}
}

func rot13(data []byte) {
	for i, c := range data {
		base := byte(0)
		if c >= 'a' && c <= 'z' {
			base = 'a'
		} else if c >= 'A' && c <= 'Z' {
			base = 'A'
		}
		if base != 0 {
			data[i] = byte((c-base+13)%26 + base)
		}
	}
}
