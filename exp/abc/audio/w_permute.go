package audio

import "code.google.com/p/rog-go/exp/abc"

type PermuteWidget struct {
	Format
	input Widget
	p     []int
	buf   Buffer

	permute func(b0, b1 Buffer, p []int)
}

func init() {
	Register("permute", wProc, map[string]abc.Socket{
		"out": abc.Socket{SamplesT, abc.Male},
		"1":   abc.Socket{SamplesT, abc.Female},
		"2":   abc.Socket{abc.StringT, abc.Female},
	}, makePermute)
}

func makePermute(status *abc.Status, args map[string]interface{}) Widget {
	w := new(PermuteWidget)
	w.Type = Float32Type
	spec := args["2"].(string)
	p := make([]int, len(spec))
	for i, c := range spec {
		if c == '.' {
			p[i] = -1
		} else {
			if c < '0' || c > '9' {
				panic("invalid permute specification: " + spec)
			}
			p[i] = c - '0'
		}
	}
	w.p = p
	return w
}

func Permute(input Widget, p []int) *PermuteWidget {
	return (&PermuteWidget{p: p}).init(input)
}

func (w *PermuteWidget) Init(inputs map[string]Widget) {
	w.init(inputs["1"])
}

func (w *PermuteWidget) init(input Widget) *PermuteWidget {
	w.input = input
	inf := input.GetFormat("out")
	w.Format = inf
	w.NumChans = len(w.p)

	for _, c := range w.p {
		if c != -1 && c < 0 || c >= inf.NumChans {
			panic("permute index out of range")
		}
	}

	switch w.Format.Layout {
	case Interleaved:
		w.permute = permuteNFloat32Buf
	case NonInterleaved:
		w.permute = permuteFloat32NBuf
	default:
		panic("bad format")
	}
	w.buf = w.AllocBuffer(0)
	debugp("permute format %v (input format %v)\n", w.Format, input.GetFormat("out"))
	return w
}

func (w *PermuteWidget) ReadSamples(b Buffer, t int64) bool {
	defer un(log("permute read %v [%v]", t, b.Len()))
	if b.Len() > w.buf.Len() {
		w.buf = w.input.GetFormat("out").AllocBuffer(b.Len())
	}
	if !w.input.ReadSamples(w.buf, t) {
		return false
	}
	w.permute(b, w.buf, w.p)
	return true
}

func permuteFloat32NBuf(b0, b1 Buffer, p []int) {
	b0.Copy(0, b1.(Float32NBuf).Permute(p), 0, b1.Len())
}

func permuteNFloat32Buf(b0, b1 Buffer, p []int) {
	buf0 := b0.(NFloat32Buf)
	buf1 := b1.(NFloat32Buf)

	nc0 := buf0.NumChans
	nc1 := buf1.NumChans
	for c := 0; c < nc0; c++ {
		if p[c] == -1 {
			for i := c; i < len(buf0.Buf); i += nc0 {
				buf0.Buf[i] = 0
			}
		} else {
			j := p[c]
			for i := c; i < len(buf0.Buf); i += nc0 {
				buf0.Buf[i] = buf1.Buf[j]
				j += nc1
			}
		}
	}
}
