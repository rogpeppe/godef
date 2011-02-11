package audio

import (
	"fmt"
)

type Buffer interface {
	Len() int
	Zero(i0, i1 int)
	Copy(i0 int, src Buffer, j0, j1 int)
	Slice(i0, i1 int) Buffer
	GetFormat() Format
}

type ContiguousFloat32Buffer interface {
	Buffer
	AsFloat32Buf() Float32Buf
	AllocFromFloat32Buf(buf Float32Buf) ContiguousFloat32Buffer
}

type Float32Buf []float32

// n channels interleaved
type NFloat32Buf struct {
	NumChans int
	Size int
	Buf Float32Buf
}

func (b Float32Buf) Len() int {
	return len(b)
}

// n separate channels
type Float32NBuf []Float32Buf

var zeros = make([]float32, 128)

func (b Float32Buf) Zero(i0, i1 int) {
	n := 0
	for b := b[i0:i1]; len(b) > 0; b = b[n:] {
		n = copy(b, zeros)
	}
}

func (b Float32Buf) GetFormat() Format {
	return Format{
		NumChans: 1,
		Type: Float32Type,
		Layout: Mono,
	}
}

func (b Float32Buf) Copy(i0 int, src Buffer, j0, j1 int) {
	copy(b[i0:], src.(Float32Buf)[j0 : j1])
}

func (b Float32Buf) Slice(i0, i1 int) Buffer {
	return b[i0 : i1]
}

func (b NFloat32Buf) Len() int {
	return b.Size
}

func (b NFloat32Buf) Zero(i0, i1 int) {
	Float32Buf(b.Buf).Zero(i0 * b.NumChans, i1 * b.NumChans)
}

func (b NFloat32Buf) GetFormat() Format {
	return Format{
		NumChans: b.NumChans,
		Type: Float32Type,
		Layout: Interleaved,
	}
}
	

func (b0 NFloat32Buf) Copy(i0 int, b1 Buffer, j0, j1 int) {
	switch b1 := b1.(type) {
	case NFloat32Buf:
		if b1.NumChans != b0.NumChans {
			panic(fmt.Sprintf("mismatched channel count (%d vs %d)", b0.NumChans, b1.NumChans))
		}
		i0 *= b0.NumChans
		j0 *= b0.NumChans
		j1 *= b0.NumChans
		copy(b0.Buf[i0:], b1.Buf[j0 : j1])

	case Float32NBuf:
		nc := b0.NumChans
		if nc != len(b1) {
			panic("mismatched channel count")
		}
		for c := 0; c < nc; c++ {
			j := c
			for _, v := range b1[c] {
				b0.Buf[j] = v
				j += nc
			}
		}

	case Float32Buf:
		if b0.NumChans != 1 {
			panic("mismatched channel count")
		}
		copy(b0.Buf[i0:], b1[j0 : j1])

	default:
		panic("unexpected buffer type")
	}
}

func (b NFloat32Buf) Slice(i0, i1 int) Buffer {
	b.Buf = b.Buf[i0 * b.NumChans : i1 * b.NumChans]
	b.Size = i1 - i0
	return b
}

func (b NFloat32Buf) AsFloat32Buf() Float32Buf {
	return b.Buf
}

func (b NFloat32Buf)  AllocFromFloat32Buf(buf Float32Buf) ContiguousFloat32Buffer {
	return NFloat32Buf{
		NumChans: b.NumChans,
		Size: len(buf) / b.NumChans,
		Buf: buf,
	}
}

func AllocNFloat32Buf(nchans, n int) NFloat32Buf {
	return NFloat32Buf{
		NumChans: nchans,
		Size: n,
		Buf: make([]float32, n * nchans),
	}
}

func (b Float32NBuf) Len() int {
	return len(b[0])
}

func (b Float32NBuf) Zero(i0, i1 int) {
	for _, c := range b {
		c.Zero(i0, i1)
	}
}

func (b0 Float32NBuf) Copy(i0 int, b1 Buffer, j0, j1 int) {
	switch b1 := b1.(type) {
	case Float32NBuf:
		if len(b1) != len(b0) {
			panic("incompatible channel count")
		}
		for i, c := range b0 {
			c.Copy(i0, b1[i], j0, j1)
		}
	case NFloat32Buf:
		nc := b1.NumChans
		if nc != len(b0) {
			panic("mismatched channel count")
		}
		size := b1.Size
		for c := 0; c < nc; c++ {
			buf := b0[c]
			j := c
			for i := 0; i < size; i++ {
				buf[i] = b1.Buf[j]
				j += nc
			}
		}
	default:
		panic("unexpected buffer type")
	}
}

func (b Float32NBuf) Slice(i0, i1 int) Buffer {
	r := make([]Float32Buf, len(b))
	for i, c := range b {
		r[i] = c[i0 : i1]
	}
	return Float32NBuf(r)
}

func (b Float32NBuf) GetFormat() Format {
	return Format{
		NumChans: len(b),
		Type: Float32Type,
		Layout: NonInterleaved,
	}
}

// each item in p represents a channel in the new buffer;
// its value is which channel it comes from. if it's -1,
// then we create a zero'd buffer
func (b0 Float32NBuf) Permute(p []int) Float32NBuf {
	r := make(Float32NBuf, len(p))
	var z Float32Buf
	for i, c := range p {
		if c == -1 {
			if z == nil {
				z = make(Float32Buf, b0.Len())
			}
			r[i] = z
		}else{
			r[i] = b0[c]
		}
	}
	return r
}

func AllocFloat32NBuf(nchans, n int) Float32NBuf {
	r := make(Float32NBuf, nchans)

	// people can't expect to use cap()
	buf := make(Float32Buf, n * nchans)
	j := 0
	for i := range r {
		r[i] = buf[j : j + n]
		j += n
	}
	return r
}


type Int16Buf []int16

func (b Int16Buf) GetFormat() Format {
	return Format{
		NumChans: 1,
		Type: Int16Type,
		Layout: Mono,
	}
}

func (b Int16Buf) Len() int {
	return len(b)
}

func (b Int16Buf) Zero(i0, i1 int) {
	for i := i0; i < i1; i++ {
		b[i] = 0
	}
}

func (b Int16Buf) Copy(i0 int, src Buffer, j0, j1 int) {
	copy(b[i0:], (src.(Int16Buf))[j0 : j1])
}

func (b Int16Buf) Slice(i0, i1 int) Buffer {
	nb := b[i0 : i1]
	return &nb
}
