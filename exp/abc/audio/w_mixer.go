package audio

import (
	"code.google.com/p/rog-go/exp/abc"
	"strconv"
)

func init() {
	Register("mix", wProc, map[string]abc.Socket{
		"out": abc.Socket{SamplesT, abc.Male},
		"1":   abc.Socket{SamplesT, abc.Female},
		"2":   abc.Socket{SamplesT, abc.Female},
	}, makeMixer)
}

type MixWidget struct {
	Format
	buf     ContiguousFloat32Buffer
	ws      []Widget
	carryOn bool
}

func makeMixer(status *abc.Status, args map[string]interface{}) Widget {
	w := new(MixWidget)
	w.Layout = Interleaved
	w.Type = Float32Type
	return w
}

func atoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return i
}

func (w *MixWidget) Init(inputs map[string]Widget) {
	defer un(log("MixWidget.Init"))
	ws := make([]Widget, len(inputs))
	for i, w := range inputs {
		ws[atoi(i)-1] = w
	}
	w.init(ws)
}

func Mixer(carryOn bool, ws []Widget) *MixWidget {
	if len(ws) == 0 {
		panic("must have more than one channel to mix")
	}
	for _, input := range ws {
		if !input.GetFormat("out").FullySpecified() {
			panic("input formats not fully specified")
		}
	}

	return (&MixWidget{carryOn: carryOn}).init(ws)
}

func (w *MixWidget) init(ws []Widget) *MixWidget {
	if len(ws) == 0 {
		panic("must have at least one channel to mix")
	}
	debugp("mixing %#v\n", ws)
	// choose best quality input, and convert all others to that.
	var best Format
	for _, input := range ws {
		f := input.GetFormat("out")
		best = best.CombineBest(f)
	}
	debugp("mixer: best format: %#v", best)
	for i, input := range ws {
		f := input.GetFormat("out")
		if !f.Eq(best) {
			ws[i] = Converter(input, best)
		}
	}
	w.ws = ws
	w.Format = best
	w.buf = w.AllocBuffer(0).(ContiguousFloat32Buffer)
	return w
}

func (w *MixWidget) ReadSamples(buf Buffer, t int64) bool {
	defer un(log("mix read %v [%v]", t, buf.Len()))
	fbuf := buf.(ContiguousFloat32Buffer)
	samples := fbuf.AsFloat32Buf()
	if len(w.ws) == 0 {
		return false
	}
	if buf.Len() > w.buf.Len() {
		w.buf = w.AllocBuffer(buf.Len()).(ContiguousFloat32Buffer)
	}
	wbuf := w.buf.Slice(0, buf.Len()).(ContiguousFloat32Buffer)
	ok := w.ws[0].ReadSamples(buf, t)
	for i, mw := range w.ws[1:] {
		if mw.ReadSamples(wbuf, t) {
			for j, s := range wbuf.AsFloat32Buf() {
				samples[j] += s
			}
		} else {
			ok = false
			w.ws[i] = nil
		}
	}
	if !ok {
		if !w.carryOn {
			w.ws = nil
			return false
		}
		// remove eof'd widgets
		j := 0
		for i, mw := range w.ws {
			if w.ws != nil {
				if i != j {
					w.ws[j] = mw
					w.ws[i] = nil
				}
				j++
			}
		}
		w.ws = w.ws[0:j]
	}
	return len(w.ws) > 0
}
