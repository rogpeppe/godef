package audio

import "code.google.com/p/rog-go/exp/abc"

type MultiplierWidget struct {
	Format
	eof    bool
	buf    ContiguousFloat32Buffer
	w0, w1 Widget
}

func init() {
	Register("multiply", wProc, map[string]abc.Socket{
		"out": abc.Socket{SamplesT, abc.Male},
		"1":   abc.Socket{SamplesT, abc.Female},
		"2":   abc.Socket{SamplesT, abc.Female},
	}, makeMultiplier)
}

func makeMultiplier(status *abc.Status, args map[string]interface{}) Widget {
	w := new(MultiplierWidget)
	w.Layout = Interleaved
	w.Type = Float32Type
	return w
}

func Multiplier(w0, w1 Widget) *MultiplierWidget {
	return (&MultiplierWidget{}).init(w0, w1)
}

func (w *MultiplierWidget) init(w0, w1 Widget) *MultiplierWidget {
	f0 := w0.GetFormat("out")
	f1 := w1.GetFormat("out")
	if !f0.Eq(f1) {
		best := f0.CombineBest(f1)
		if !f0.Eq(best) {
			w0 = Converter(w0, best)
		}
		if !f1.Eq(best) {
			w1 = Converter(w1, best)
		}
		w.Format = best
	} else {
		w.Format = f0
	}
	w.w0 = w0
	w.w1 = w1
	w.buf = w.AllocBuffer(0).(ContiguousFloat32Buffer)
	return w
}

func (w *MultiplierWidget) Init(inputs map[string]Widget) {
	w.init(inputs["1"], inputs["2"])
}

func (w *MultiplierWidget) ReadSamples(b Buffer, t int64) bool {
	defer un(log("mult read %v [%v]", t, b.Len()))

	buf0 := b.(ContiguousFloat32Buffer)

	if w.eof {
		return false
	}
	if buf0.Len() > w.buf.Len() {
		w.buf = w.AllocBuffer(buf0.Len()).(ContiguousFloat32Buffer)
	}
	buf1 := w.buf.Slice(0, buf0.Len()).(ContiguousFloat32Buffer)
	ok0 := w.w0.ReadSamples(buf0, t)
	ok1 := w.w1.ReadSamples(buf1, t)
	if ok0 && ok1 {
		b0 := buf0.AsFloat32Buf()
		b1 := buf1.AsFloat32Buf()
		for j, s := range b1 {
			b0[j] *= s
		}
		return true
	}
	w.eof = true
	return false
}
