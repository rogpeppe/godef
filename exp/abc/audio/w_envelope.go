package audio

import (
	"code.google.com/p/rog-go/exp/abc"
	"strconv"
)

type EnvelopeWidget struct {
	Format
	read           func(b Float32Buf, t int64) int64
	t0, a, d, s, r Time
	sustainlev     float32
}

func init() {
	Register("envelope", wOutput, map[string]abc.Socket{
		"start": abc.Socket{TimeT, abc.Female},
		"a":     abc.Socket{TimeT, abc.Female},
		"d":     abc.Socket{TimeT, abc.Female},
		"s":     abc.Socket{TimeT, abc.Female},
		"r":     abc.Socket{TimeT, abc.Female},
		"level": abc.Socket{abc.StringT, abc.Female},
		"out":   abc.Socket{SamplesT, abc.Male},
	}, makeEnvelope)
}

func getDefault(m map[string]interface{}, key string, deflt interface{}) interface{} {
	if v := m[key]; v != nil {
		return v
	}
	return deflt
}

var zerot = Time{0, false}

func makeEnvelope(status *abc.Status, args map[string]interface{}) Widget {
	w := &EnvelopeWidget{}

	w.NumChans = 1
	w.Type = Float32Type
	w.Layout = Interleaved
	w.a = getDefault(args, "a", zerot).(Time)
	w.d = getDefault(args, "d", zerot).(Time)
	w.s = getDefault(args, "s", zerot).(Time)
	w.r = getDefault(args, "r", zerot).(Time)
	w.t0 = getDefault(args, "start", zerot).(Time)
	lev := getDefault(args, "level", "1").(string)
	level, err := strconv.Atof64(lev)
	if err != nil {
		panic("bad level: " + lev)
	}
	w.sustainlev = float32(level)
	return w
}

func Envelope(t0 int64, rate int, attack, decay, sustain, release int64, sustainlev float32) *EnvelopeWidget {
	defer un(log("Envelope"))
	w := &EnvelopeWidget{}
	w.NumChans = 1
	w.Type = Float32Type
	w.Layout = Interleaved
	w.Rate = rate

	w.a.t = attack
	w.d.t = decay
	w.s.t = sustain
	w.r.t = release
	w.sustainlev = sustainlev
	return w.init()
}

func (w *EnvelopeWidget) Init(_ map[string]Widget) {
	w.init()
}

func (w *EnvelopeWidget) init() *EnvelopeWidget {
	attack := w.TimeToSamples(w.a)
	decay := w.TimeToSamples(w.d)
	sustain := w.TimeToSamples(w.s)
	release := w.TimeToSamples(w.r)
	t0 := w.TimeToSamples(w.t0)
	sustainlev := w.sustainlev

	attackdelta := 1 / float32(attack)
	attackt0 := t0
	attackt1 := t0 + attack

	decaydelta := (sustainlev - 1) / float32(decay)
	decayt1 := attackt1 + decay

	sustaint1 := decayt1 + sustain

	releasedelta := (0 - sustainlev) / float32(release)
	releaset1 := sustaint1 + release

	releasefn := w.until(releaset1, func(buf Float32Buf, p0 int64) {
		slope(buf, p0, sustaint1, sustainlev, releasedelta)
	}, finished)

	sustainfn := w.until(sustaint1, func(buf Float32Buf, p0 int64) {
		level(buf, p0, sustainlev)
	}, releasefn)

	decayfn := w.until(decayt1, func(buf Float32Buf, p0 int64) {
		slope(buf, p0, attackt1, 1, decaydelta)
	}, sustainfn)

	attackfn := w.until(attackt1, func(buf Float32Buf, p0 int64) {
		slope(buf, p0, attackt0, 0, attackdelta)
	}, decayfn)

	initialfn := func(buf Float32Buf, p0 int64) int64 {
		p1 := p0 + int64(len(buf))
		if p1 < t0 {
			buf.Zero(0, len(buf))
			return p1
		}
		if p0 < t0 {
			n := int(t0 - p0)
			buf.Zero(0, n)
			buf = buf[n:]
			p0 += int64(n)
		}
		w.read = attackfn
		return attackfn(buf, p0)
	}

	w.read = initialfn
	return w
}

func (w *EnvelopeWidget) SetFormat(f Format) {
	defer un(log("env SetFormat %v\n", f))
	w.Format = f
}

func (w *EnvelopeWidget) ReadSamples(b Buffer, t int64) (r bool) {
	defer un(log("env read %v [%v]\n", t, b.Len()), &r)
	buf := b.(NFloat32Buf).Buf
	n := int(w.read(buf, t) - t)
	switch {
	case n <= 0:
		return false
	case n < len(buf):
		b.Zero(n, len(buf))
	}
	return true
}

func slope(buf Float32Buf, p0 int64, t0 int64, start, delta float32) {
	lev := start + float32(p0-t0)*delta
	for i := range buf {
		buf[i] = lev
		lev += delta
	}
}

func level(buf Float32Buf, t int64, level float32) {
	for i := range buf {
		buf[i] = level
	}
}

func finished(buf Float32Buf, t int64) int64 {
	return t
}

// until returns a sample reader function that uses fill to
// fill the buffer until time t, whereupon the next
// function will take over.
//
func (w *EnvelopeWidget) until(t1 int64, fill func(Float32Buf, int64), next func(Float32Buf, int64) int64) func(buf Float32Buf, p0 int64) int64 {
	return func(buf Float32Buf, p0 int64) int64 {
		defer un(log("until %v [end %v]", p0, t1))
		p1 := p0 + int64(len(buf))
		if p0 < t1 {
			debugp("filling some")
			fillbuf := buf
			if p1 > t1 {
				fillbuf = buf[0:int(t1-p0)]
				buf = buf[len(fillbuf):]
			}
			fill(fillbuf, p0)
			p0 += int64(len(fillbuf))
		}
		if p1 >= t1 {
			w.read = next
			return next(buf, p0)
		}
		return p1
	}
}
