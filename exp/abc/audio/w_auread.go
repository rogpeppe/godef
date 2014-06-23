package audio

import (
	"code.google.com/p/rog-go/exp/abc"
)

func init() {
	abc.Register("auread", map[string]abc.Socket{
		"out": abc.Socket{SamplesT, abc.Male},
		"1":   abc.Socket{basic.Fd, abc.Female},
	}, makeRead)
}

func makeRead(args map[string]interface{}) abc.Widget {
	var r sampleReader
	r.Init(args["1"].(basic.Fd).GetReader())
	renderfn = func(samples []float32, atTime int64) {
		r.SampleRead(samples, atTime)
	}
	args["out"].(chan interface{}) <- &node{renderfn, nil}
	return nil
}

type sampleReader struct {
	r   io.Reader
	t   int64
	buf []byte
}

func (r *sampleReader) SampleRead(samples []float32, atTime int64) {
	ns := 0
	if r.r != nil {
		need := len(samples) * bytesPerSample
		if need > len(r.buf) {
			r.buf = make([]byte, need)
		}
		if atTime > t {
			skip(r.r, atTime-t)
		}
		n, err := io.ReadAtLeast(r.r, buf, need)
		_, ns = bytes2samples(r.buf[0:n], samples)
		if n < need {
			r.r = nil
		}
	}

	// fill any unused space with silence
	for ; ns < len(samples); ns++ {
		samples[ns] = 0
	}
}

func (r *sampleReader) Init(ior io.Reader) *sampleReader {
	r.r = ior
	r.t = 0
}

const bytesPerSample = 2
const maxSample = 1<<(8*bytesPerSample) - 1

func bytes2samples(b []byte, s []float32) (nb int, ns int) {
	if len(b)&1 == 1 {
		b = b[0 : len(b)-1]
	}
	j := 0
	for i := 0; i < len(b); i += bytesPerSample {
		s[j] = float32(int(b[i])|int(b[i+1])<<8-0x7fff) / 0x8000
		j++
	}
	return len(b), j
}
