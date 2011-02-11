package audio

import (
	"fmt"
)

type ConverterWidget struct {
	outfmt Format
	infmt Format
	input Widget
}

// conversions, in order of application
// type - float32 -> int16 & vice versa (punt for now)
// num channels - use permute
// layout
// sample rate - punt for now

func Converter(input Widget, f Format) (w *ConverterWidget) {
	defer un(log("converter", f), &w)
	w = &ConverterWidget{}
	w.outfmt = f
	return w.init(input)
}

func (w *ConverterWidget) Init(inputs map[string]Widget) {
	w.init(inputs["1"])
}

func (w *ConverterWidget) init(input Widget) *ConverterWidget {
	w.infmt = input.GetFormat("out")
	w.input = input

	if !w.outfmt.FullySpecified() || !w.infmt.FullySpecified() {
		panic(fmt.Sprintf("formats not fully specified, out %#v; in %#v", w.outfmt, w.infmt))
	}
	if w.infmt.Eq(w.outfmt) {
		return w
	}
	if w.infmt.Type != w.outfmt.Type {
		panic("no type conversions yet")
	}
	switch {
	case w.infmt.NumChans == w.outfmt.NumChans:
		break

	case w.infmt.NumChans > w.outfmt.NumChans:
		// if there are fewer output chans that input chans, then just
		// discard excess input chans. TODO: add the
		// extraneous channels into the mix.
		p := make([]int, w.outfmt.NumChans)
		for i := range p {
			p[i] = i
		}
		w.input = Permute(w.input, p)

	case w.infmt.NumChans < w.outfmt.NumChans:
		// if there are more output chans than input chans,
		// replicate whole multiples of the input chans
		// across the output. any remainder gets zeroed.
		p := make([]int, w.outfmt.NumChans)
		nreps := w.infmt.NumChans / w.infmt.NumChans
		k := 0
		for i := 0; i < nreps; i++ {
			for j := 0; j < w.infmt.NumChans; j++ {
				p[k] = j
				k++
			}
		}
		for ; k < w.outfmt.NumChans; k++ {
			p[k] = -1
		}
		w.input = Permute(w.input, p)
	}

	w.input = LayoutConverter(w.input, w.outfmt.Layout)
	if w.infmt.Rate != w.outfmt.Rate {
		panic("no sample rate conversions yet")
	}
	if !w.input.GetFormat("out").Eq(w.outfmt) {
		panic(fmt.Sprintf("conversion not performed (orig: %#v; in: %#v; out: %#v)", w.infmt, w.input.GetFormat("out"), w.outfmt))
	}
	return w
}

func (w *ConverterWidget) ReadSamples(b Buffer, t int64) bool {
	defer un(log("converter read %v [%v]", t, b.Len()))
	return w.input.ReadSamples(b, t)
}

func (w *ConverterWidget) GetFormat(name string) (f Format) {
	switch name {
	case "out":	
		f = w.outfmt
	case "0":
		f = w.infmt
	default:
		panic("unknown name")
	}
	return
}

type LayoutConverterWidget struct {
	Format
	layout int
	input Widget
	buf Buffer
}

func LayoutConverter(input Widget, layout int) Widget {
	if  input.GetFormat("out").Layout == layout {
		return input
	}
	w := &LayoutConverterWidget{}
	w.Format = input.GetFormat("out")
	w.input = input
	w.layout = layout
	w.buf = w.AllocBuffer(0)
	return w
}


func (w *LayoutConverterWidget) GetFormat(name string) Format {
	f := w.Format
	if name == "out" {
		f.Layout = w.layout
	}
	return f
}

func (w *LayoutConverterWidget) Init(_ map[string]Widget) {
	panic("Init call not allowed")
}

func (w *LayoutConverterWidget) ReadSamples(b Buffer, t int64) bool {
	defer un(log("layout read %v [%d]", t, b.Len()))
	if b.Len() > w.buf.Len() {
		w.buf = w.AllocBuffer(b.Len())
	}
	if !w.input.ReadSamples(w.buf, t) {
		return false
	}
	b.Copy(0, w.buf, 0, b.Len())
	return true
}
