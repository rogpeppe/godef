package audio

import (
	"code.google.com/p/rog-go/exp/abc"
	"fmt"
	"strconv"
	"testing"
)

func xTestReadWriteWav(t *testing.T) {
	defer unregistertests(registertests(t))
	abc.Exec(`
		readwav "test.wav" -out $>wav
		wave 440 -out $>tone
		level 0.3 -out $>lev
		multiply $tone $lev -out $>quiettone
		mix $wav $quiettone -out $>mixed
		writewav $mixed "test2.wav"
	`)
}

func xTestParser(t *testing.T) {
	abc.Exec2(`
		readwav "test.wav" -out $>wav
		wave 440 -out $>tone
		level 0.3 -out $>lev
		multiply $tone $lev -out $>quiettone
		mix $wav $quiettone -out $>mixed
		writewav $mixed "test2.wav"
	`)
}

func TestParserWithPipes(t *testing.T) {
	fmt.Printf("testing parser\n")
	abc.Exec2(`
		wave 440 |
			multiply {envelope -s {time "20s"} -r {time "1s"} -level 0.3} |
			permute 00 |
			phaser | writewav "test2.wav"
//		chord = wave 440 | multiply {level 0.3} | mix {wave 293.333 | multiply {level 0.3}}
//		readwav "test.wav" |
//			mix $chord |
//			writewav "test2.wav"
	`)
}

type CompareWidget struct {
	Format
	bufsize int
	w0, w1  Widget
	t       *testing.T
}

func registertests(t *testing.T) *testing.T {
	Register("compare", wOutput, map[string]abc.Socket{
		"1": abc.Socket{SamplesT, abc.Female},
		"2": abc.Socket{SamplesT, abc.Female},
	}, func(status *abc.Status, args map[string]interface{}) Widget {
		return makeCompareWidget(t, status, args)
	})
	return t
}

func unregistertests(t *testing.T) {
	abc.Unregister("compare")
}

func makeCompareWidget(t *testing.T, status *abc.Status, args map[string]interface{}) Widget {
	w := new(CompareWidget)
	w.Layout = Interleaved
	w.Type = Float32Type
	w.t = t
	if args["bufsize"] != nil {
		w.bufsize, _ = strconv.Atoi(args["bufsize"].(string))
	}
	if w.bufsize <= 0 {
		w.bufsize = 1024
	}
	return w
}

func (w *CompareWidget) Init(inputs map[string]Widget) {
	r0 := w.w0
	r1 := w.w1
	if !r0.GetFormat("out").Eq(r1.GetFormat("out")) {
		w.t.Fatal("differing formats")
	}

	buf0 := make(Float32Buf, w.bufsize)
	buf1 := make(Float32Buf, w.bufsize)
	p := int64(0)
	var eof0, eof1 bool
	for {
		eof0 = !r0.ReadSamples(buf0, p)
		eof1 = !r1.ReadSamples(buf1, p)
		if eof0 || eof1 {
			break
		}
		for j, s := range buf0 {
			if s != buf1[j] {
				w.t.Fatalf("difference at %v (%v vs %v)\n", p+int64(j), s, buf1[j])
			}
		}
		p += int64(w.bufsize)
	}
	if eof0 != eof1 {
		if eof0 {
			w.t.Fatalf("premature eof on 0\n")
		} else {
			w.t.Fatalf("premature eof on 1\n")
		}
	}
}

func (w *CompareWidget) ReadSamples(_ Buffer, _ int64) bool {
	return false
}

func testequal(t *testing.T, a, b []byte, msg string) {
	for i, x := range a {
		if b[i] != x {
			p := i / 2 * 2
			t.Log(a[p:p+2], b[p:p+2])
			t.Fatalf("bytes differ at %d: %v", i, msg)
		}
	}
}

// round trip conversion int16->float32->int16
func TestConversion(t *testing.T) {
	origdata := make([]byte, 2*65536)
	samples := make([]float32, 65536)
	for i := 0; i < len(origdata); i += 2 {
		origdata[i] = byte(i & 0xff)
		origdata[i+1] = byte(i >> 8)
	}
	data := make([]byte, len(origdata))
	copy(data, origdata)
	int16tofloat32le(samples, data)
	float32toint16le(data, samples)
	testequal(t, origdata, data, "little endian")
}
