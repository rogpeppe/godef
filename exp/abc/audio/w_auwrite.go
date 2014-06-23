package audio

import (
	"code.google.com/p/rog-go/exp/abc"
)

func init() {
	abc.Register("auwrite", map[string]abc.Socket{
		"out": abc.Socket{basic.Fd, abc.Male},
		"1":   abc.Socket{SamplesT, abc.Female},
	}, makeWrite)
}

func makeWrite(args map[string]interface{}) abc.Widget {
	out := basic.NewFd()
	args["out"].(chan interface{}) <- out
	w := out.GetWriter(nil)
	samples := make([]float32, 1024)
	buf := make([]byte, len(samples)*bytesPerSample)
	node := args["1"].(*node)

	t := int64(0)
	for {
		node.render(samples, t)
		n := samples2bytes(samples, buf)
		if _, err := w.Write(buf[0:n]); err != nil {
			break
		}
	}
}

func samples2bytes(s []float32, b []byte) int {
	i := 0
	for _, x := range s {
		y := int(x*0x8000) + 0x7fff
		switch {
		case y < 0:
			y = 0
		case y > 0xffff:
			y = 0xffff
		}
		b[i] = byte(y)
		b[i+1] = byte(y >> 8)
		i += 2
	}
	return i
}
