package audio

import (
	"code.google.com/p/rog-go/exp/abc"
	"fmt"
	"math"
	"os"
	"strconv"
)

// phaser stolen from audacity.
// audacity-src-1.3.12-beta/src/effects/Phaser.cpp

type PhaserWidget struct {
	Format
	input Widget

	// parameters
	freq, startphase, fb, drywet float32
	depth, stages                int

	// statically derived values
	lfoskip float32

	state []PhaserState
}

type PhaserState struct {
	old         []float32
	skipcount   int64 // ??
	gain, fbout float32
}

const (
	phaserlfoshape = 4
	lfoskipsamples = 20
)

func init() {
	Register("phaser", wProc, map[string]abc.Socket{
		"out":      abc.Socket{SamplesT, abc.Male},
		"1":        abc.Socket{SamplesT, abc.Female},
		"freq":     abc.Socket{abc.StringT, abc.Female},
		"phase":    abc.Socket{abc.StringT, abc.Female},
		"feedback": abc.Socket{abc.StringT, abc.Female},
		"depth":    abc.Socket{abc.StringT, abc.Female},
		"stages":   abc.Socket{abc.StringT, abc.Female},
		"wet":      abc.Socket{abc.StringT, abc.Female},
	}, makePhaser)
}

func makePhaser(status *abc.Status, args map[string]interface{}) Widget {
	w := new(PhaserWidget)
	w.Layout = NonInterleaved
	w.Type = Float32Type

	w.freq = getFloat32(args, "freq", 1, 40, 4) / 10
	w.startphase = getFloat32(args, "phase", 0, 359, 0) * 180 / math.Pi
	w.fb = getFloat32(args, "feedback", -100, 100, 0) / 100
	w.depth = getInt(args, "depth", 0, 255, 100)
	w.stages = getInt(args, "stages", 2, 24, 2)
	w.drywet = getFloat32(args, "wet", 0, 255, 128) / 255
	return w
}

func (w *PhaserWidget) Init(inputs map[string]Widget) {
	w.input = inputs["1"]
	w.Format = w.input.GetFormat("out")
	w.state = make([]PhaserState, w.NumChans)
	w.lfoskip = w.freq * 2 * math.Pi / float32(w.Rate)
	for i := range w.state {
		w.state[i].old = make([]float32, w.stages)
	}
}

func (w *PhaserWidget) ReadSamples(b Buffer, t int64) bool {
	defer un(log("phaser read %d [%d]", t, b.Len()))
	if !w.input.ReadSamples(b, t) {
		return false
	}
	buffers := b.(Float32NBuf)
	phase := w.startphase
	for c, buffer := range buffers {
		state := &w.state[c]
		for i, in := range buffer {
			if state.skipcount%lfoskipsamples == 0 {
				// compute sine between 0 and 1
				// TODO: truncate skipcount, modify lfoskip
				state.gain = (1 + cos32(float32(state.skipcount)*w.lfoskip+phase)) / 2
				// change lfo shape
				state.gain = float32((math.Exp(float64(state.gain*phaserlfoshape)) - 1) /
					(math.Exp(phaserlfoshape) - 1))
				state.gain = 1 - state.gain/255*float32(w.depth) // attenuate the lfo
			}
			state.skipcount++
			// phasing routine
			m := in + state.fbout*w.fb
			g := state.gain
			for j, tmp := range state.old {
				v := g*tmp + m
				state.old[j] = v
				m = tmp - g*v
			}
			state.fbout = m
			out := m*w.drywet + in*(1-w.drywet)
			switch {
			case out < -1.0:
				out = -1.0
			case out > 1.0:
				out = 1.0
			}
			buffer[i] = out
		}
		phase += math.Pi
	}
	return true
}

func cos32(x float32) float32 {
	return float32(math.Cos(float64(x)))
}
func getInt(args map[string]interface{}, name string, min, max, deflt int) int {
	x := deflt
	if v := args[name]; v != nil {
		var err os.Error
		x, err = strconv.Atoi(v.(string))
		if err != nil {
			panic("invalid integer on parameter '" + name + "'")
		}
		if x < min || x > max {
			panic(fmt.Sprintf("parameter '%s' out of range [%v, %v]", name, min, max))
		}
	}
	return x
}

func getFloat32(args map[string]interface{}, name string, min, max, deflt float32) float32 {
	x := deflt
	if v := args[name]; v != nil {
		x1, err := strconv.Atof64(v.(string))
		if err != nil {
			panic("invalid integer on parameter '" + name + "'")
		}
		x = float32(x1)
		if x < min || x > max {
			panic(fmt.Sprintf("parameter '%s' out of range [%v, %v]", name, min, max))
		}
	}
	return x
}
