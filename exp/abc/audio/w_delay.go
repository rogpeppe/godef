package audio

import (
	"code.google.com/p/rog-go/exp/abc"
	"fmt"
)

// a simplified, 1-reader ring buffer that can
// deal with reads greater than the buffer size.

type DelayWidget struct {
	Format
	buf    Buffer
	size   int
	delay  Time
	r0     int64 // time of sample in buffer
	input  Widget
	eofpos int64
	eof    bool
}

func init() {
	Register("delay", wProc, map[string]abc.Socket{
		"out": abc.Socket{SamplesT, abc.Male},
		"1":   abc.Socket{SamplesT, abc.Female},
		"2":   abc.Socket{TimeT, abc.Female},
	}, makeDelay)
}

func makeDelay(status *abc.Status, args map[string]interface{}) Widget {
	w := new(DelayWidget)
	w.delay = args["2"].(Time)
	return w
}

func Delay(input Widget, delay int) Widget {
	w := &DelayWidget{}
	w.delay = Time{int64(delay), false}
	return w.init(input)
}

func (w *DelayWidget) Init(inputs map[string]Widget) {
	w.init(inputs["1"])
}

func (w *DelayWidget) init(input Widget) *DelayWidget {
	w.Format = input.GetFormat("out")
	w.size = int(w.TimeToSamples(w.delay))
	w.buf = w.AllocBuffer(w.size)
	w.eofpos = 1<<63 - 1
	w.input = input
	return w
}

func (w *DelayWidget) ReadSamples(samples Buffer, p0 int64) bool {
	defer un(log("delay read %v [%v], read fmt %v, buf fmt %v", p0, samples.Len(), samples.GetFormat(), w.buf.GetFormat()))
	if p0 >= w.eofpos {
		return false
	}
	n := samples.Len()
	if w.size+n > w.buf.Len() {
		nbuf := w.AllocBuffer(w.size + n)
		nbuf.Copy(0, w.buf, 0, w.size)
		w.buf = nbuf
	}
	p1 := p0 + int64(n)
	if p0 != w.r0 {
		panic(fmt.Sprintf("read at wrong time (expected %v, got %v)", w.r0, p0))
	}
	off0 := int(w.r0 % int64(w.size))
	if n > w.size {
		// read is larger than buffer size
		ringCopy(samples, w.buf, off0, w.size, w.size)
		ok := !w.eof && w.input.ReadSamples(w.buf.Slice(0, n), p0)
		if ok {
			samples.Copy(w.size, w.buf, 0, n-w.size)

			// copy one delay's worth of samples to start of buffer
			w.buf.Copy(0, w.buf, n-w.size, n)
		} else {
			samples.Zero(w.size, n)
			w.eofpos = p0
			w.eof = true
		}
	} else {
		// read is smaller than buffer size
		ringCopy(samples, w.buf, off0, n, w.size)
		if !w.eof {
			off1 := (off0 + w.size) % w.size
			ok := w.input.ReadSamples(w.buf.Slice(off1, off1+n), p0)
			if ok {
				// if it wraps, copy the overhanging tail to the start of the buffer
				if off1+n > w.size {
					w.buf.Copy(0, w.buf, w.size, off1+n)
				}
			} else {
				w.eofpos = p0 + int64(w.size)
				w.eof = true
			}
		}
	}
	w.r0 = p1
	return true
}

func ringCopy(dst, samples Buffer, off, n, size int) {
	if off+n <= size {
		dst.Copy(0, samples, off, off+n)
	} else {
		gap := size - off
		dst.Copy(0, samples, off, off+gap)
		dst.Copy(gap, samples, 0, n-gap)
	}
}
