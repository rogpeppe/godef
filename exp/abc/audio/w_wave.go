package audio

import (
	"math"
	"strconv"
	"code.google.com/p/rog-go/exp/abc"
)

type waveWidget struct {
	freq    float64
	samples []float32
	Format
}

func SinWave(freq float64, rate int) (w *waveWidget) {
	w = &waveWidget{}
	w.Rate = rate
	w.NumChans = 1
	w.Type = Float32Type
	w.Layout = Interleaved
	w.freq = freq
	return w.init()
}

func init() {
	Register("wave", wInput, map[string]abc.Socket{
		"out": abc.Socket{SamplesT, abc.Male},
		"1":   abc.Socket{abc.StringT, abc.Female},
	}, makeWave)
	Register("level", wInput, map[string]abc.Socket{
		"out": abc.Socket{SamplesT, abc.Male},
		"1":   abc.Socket{abc.StringT, abc.Female},
	}, makeLevel)
}

func makeWave(status *abc.Status, args map[string]interface{}) Widget {
	freq, err := strconv.Atof64(args["1"].(string))
	if err != nil {
		panic("bad frequency")
	}
	w := &waveWidget{}
	w.freq = freq
	w.NumChans = 1
	w.Type = Float32Type
	w.Layout = Interleaved
	return w
}

func (w *waveWidget) Init(_ map[string]Widget) {
	w.init()
}

func (w *waveWidget) init() *waveWidget {
	n := int(float64(w.Rate) / w.freq)
	w.samples = make([]float32, n)
	for i := 0; i < n; i++ {
		w.samples[i] = float32(math.Sin((2 * math.Pi) / float64(n) * float64(i)))
	}
	return w
}

func (w *waveWidget) SetFormat(f Format) {
	w.Format = f
}

func CustomWave(rate int, samples []float32) (w *waveWidget) {
	w = &waveWidget{}
	w.samples = samples
	w.Rate = rate
	w.NumChans = 1
	w.Type = Float32Type
	w.Layout = Interleaved
	return w
}

func (w *waveWidget) ReadSamples(b Buffer, t int64) bool {
	defer un(log("wave read %v [%v]", t, b.Len()))
	buf := b.(NFloat32Buf).Buf
	start := int(t % int64(len(w.samples)))
	for i := 0; i < len(buf); i++ {
		buf[i] = w.samples[(i+start)%len(w.samples)]
	}
	return true
}

type levelWidget struct {
	waveWidget
}

func makeLevel(status *abc.Status, args map[string]interface{}) Widget {
	level, err := strconv.Atof64(args["1"].(string))
	if err != nil {
		panic("bad frequency")
	}
	return Level(float32(level), 0)
}

func Level(level float32, rate int) *levelWidget {
	return &levelWidget{*CustomWave(rate, []float32{level})}
}

func (w *levelWidget) Init(_ map[string]Widget) {
}

func (w *levelWidget) ReadSamples(b Buffer, _ int64) bool {
	buf := b.(NFloat32Buf).Buf
	v := w.samples[0]
	for i := range buf {
		buf[i] = v
	}
	return true
}
