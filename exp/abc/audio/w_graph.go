package audio

import (
	"code.google.com/p/rog-go/exp/abc"
	"bufio"
	"fmt"
	"os"
)

type GraphWidget struct {
	Format
	input Widget
}

func init() {
	Register("graph", wOutput, map[string]abc.Socket{
		"1": abc.Socket{SamplesT, abc.Female},
	}, makeGraph)
}

func makeGraph(status *abc.Status, args map[string]interface{}) Widget {
	w := new(GraphWidget)
	w.Layout = Interleaved
	w.Type = Float32Type
	return w
}

func (w *GraphWidget) Init(inputs map[string]Widget) {
	stdout := bufio.NewWriter(os.Stdout)
	printf := func(f string, args ...interface{}) {
		fmt.Fprintf(stdout, f, args...)
	}
	w.input = inputs["1"]
	w.Format = w.input.GetFormat("out")
	printf("sample")
	for i := 0; i < w.NumChans; i++ {
		printf(" c%d", i)
	}
	printf("\n")
	const bufsize = 64
	buf := w.AllocBuffer(bufsize).(NFloat32Buf)
	//	ibuf := make([]int16, bufsize*w.NumChans)
	t := int64(0)
	for w.input.ReadSamples(buf, t) {
		//		float32toint16(ibuf, buf)
		j := 0
		for i := 0; i < bufsize; i++ {
			printf("%d", t)
			for c := 0; c < w.NumChans; c++ {
				printf(" %v", buf.Buf[j])
				j++
			}
			t++
			printf("\n")
		}
	}
	stdout.Flush()
}

func float32toint16(data []int16, samples []float32) {
	j := 0
	for _, s := range samples {
		s *= 0x7fff
		switch {
		case s > 0x7fff:
			s = 0x7fff
		case s < -0x8000:
			s = -0x8000
		case s > 0:
			s += 0.5
		case s < 0:
			s -= 0.5
		}
		data[j] = int16(s)
		j++
	}
}

func (w *GraphWidget) ReadSamples(_ Buffer, _ int64) bool {
	return false
}
