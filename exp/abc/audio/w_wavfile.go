package audio

import (
	"code.google.com/p/rog-go/exp/abc"
	"os"
	"fmt"
	"io"
	"encoding/binary"
	"reflect"
	"strings"
)

func init() {
	Register("readwav", wInput, map[string]abc.Socket{
		"out": abc.Socket{SamplesT, abc.Male},
		"1":   abc.Socket{abc.StringT, abc.Female},
	}, makeWavReader)
	Register("writewav", wOutput, map[string]abc.Socket{
		"1": abc.Socket{SamplesT, abc.Female},
		"2": abc.Socket{abc.StringT, abc.Female},
	}, makeWavWriter)
}

type Rerror string

type fileOffset int64 // help prevent mixing of file offsets and sample counts

type WavReader struct {
	fd             *os.File
	end            fileOffset
	offset         fileOffset
	bytesPerSample int
	Format
	bytesPerFrame int
	eof           bool

	buf []byte
	cvt func([]float32, []byte)
}

type dataChunk chunkHeader

type WavWriter struct {
	Format
	fd *os.File
}

func makeWavReader(status *abc.Status, args map[string]interface{}) Widget {
	defer un(log("makeWavReader"))
	w, err := OpenWavReader(args["1"].(string))
	if w == nil {
		panic(err.String())
	}
	return w
}

func (w *WavReader) Init(_ map[string]Widget) {
}

// 0         4   ChunkID          Contains the letters "RIFF" in ASCII form
//                                (0x52494646 big-endian form).
// 4         4   ChunkSize        36 + SubChunk2Size, or more precisely:
//                                4 + (8 + SubChunk1Size) + (8 + SubChunk2Size)
//                                This is the size of the rest of the chunk
//                                following this number.  This is the size of the
//                                entire file in bytes minus 8 bytes for the
//                                two fields not included in this count:
//                                ChunkID and ChunkSize.
// 8         4   Format           Contains the letters "WAVE"
//                                (0x57415645 big-endian form).
//
//
// The "WAVE" format consists of two subchunks: "fmt " and "data":
// The "fmt " subchunk describes the sound data's format:
//
// 12        4   Subchunk1ID      Contains the letters "fmt "
//                                (0x666d7420 big-endian form).
// 16        4   Subchunk1Size    16 for PCM.  This is the size of the
//                                rest of the Subchunk which follows this number.
// 20        2   AudioFormat      PCM = 1 (i.e. Linear quantization)
//                                Values other than 1 indicate some
//                                form of compression.
// 22        2   NumChannels      Mono = 1, Stereo = 2, etc.
// 24        4   SampleRate       8000, 44100, etc.
// 28        4   ByteRate         == SampleRate * NumChannels * BitsPerSample/8
// 32        2   BlockAlign       == NumChannels * BitsPerSample/8
//                                The number of bytes for one sample including
//                                all channels. I wonder what happens when
//                                this number isn't an integer?
// 34        2   BitsPerSample    8 bits = 8, 16 bits = 16, etc.
//           2   ExtraParamSize   if PCM, then doesn't exist
//           X   ExtraParams      space for extra parameters
//
// The "data" subchunk contains the size of the data and the actual sound:
//
// 36        4   Subchunk2ID      Contains the letters "data"
//                                (0x64617461 big-endian form).
// 40        4   Subchunk2Size    == NumSamples * NumChannels * BitsPerSample/8
//                                This is the number of bytes in the data.
//                                You can also think of this as the size
//                                of the read of the subchunk following this
//                                number.
// 44        *   Data             The actual sound data.

type chunkHeader struct {
	ChunkID   uint32
	ChunkSize uint32
}

type riffChunk struct {
	ChunkSize uint32
	Format    uint32
}

type fmtChunk struct {
	chunkHeader
	AudioFormat   int16
	NumChannels   int16
	SampleRate    int32
	ByteRate      int32
	BlockAlign    int16
	BitsPerSample int16
}

func sizeof(x interface{}) int {
	return binary.TotalSize(reflect.NewValue(x))
}

func OpenWavReader(filename string) (r *WavReader, err os.Error) {
	defer func() {
		switch s := recover().(type) {
		case nil:

		case Rerror:
			err = os.NewError(string(s))

		default:
			panic(s)
		}
	}()
	fd, err := os.Open(filename)
	if fd == nil {
		return nil, err
	}

	var hd uint32
	binread(fd, binary.BigEndian, &hd)
	var endian binary.ByteOrder = binary.LittleEndian
	switch hd {
	case str4("RIFF", binary.BigEndian):
		endian = binary.LittleEndian
	case str4("RIFX", binary.BigEndian):
		endian = binary.BigEndian
	default:
		panic(Rerror("unknown wav header"))
	}
	var c0 riffChunk
	binread(fd, endian, &c0)
	if c0.Format != str4("WAVE", endian) {
		panic(Rerror(fmt.Sprintf("bad format %x", c0.Format)))
	}

	var c1 fmtChunk
	binread(fd, endian, &c1)
	if c1.ChunkID != str4("fmt ", endian) {
		panic(Rerror("unexpected chunk header id"))
	}
	if c1.AudioFormat != 1 {
		panic(Rerror("unknown audio format"))
	}

	var c2 dataChunk
	binread(fd, endian, &c2)
	if c2.ChunkID != str4("data", endian) {
		panic(Rerror("no data chunk found"))
	}

	r = &WavReader{fd: fd}
	r.cvt = cvtFns[endian][int(c1.BitsPerSample)]
	if r.cvt == nil {
		panic(Rerror("unsupported bits per sample"))
	}
	r.NumChans = int(c1.NumChannels)
	r.Rate = int(c1.SampleRate)
	r.Type = Float32Type
	r.Layout = Interleaved
	r.bytesPerSample = int(c1.BitsPerSample) / 8
	r.bytesPerFrame = r.bytesPerSample * r.NumChans
	r.end = fileOffset(c2.ChunkSize)

	return r, nil
}

var cvtFns = map[binary.ByteOrder]map[int]func([]float32, []byte){
	binary.LittleEndian: map[int]func([]float32, []byte){
		16: int16tofloat32le,
	},
	binary.BigEndian: map[int]func([]float32, []byte){
		16: int16tofloat32be,
	},
}

func (r *WavReader) ReadSamples(b Buffer, p int64) bool {
	if r.eof {
		return false
	}
	defer un(log("wav read %v [%d]", p, b.Len()))
	o0 := fileOffset(p * int64(r.bytesPerFrame))
	if o0 != r.offset {
		r.fd.Seek(int64(o0-r.offset), 1)
		r.offset = o0
	}
	if r.offset >= r.end {
		return false
	}
	samples := b.(ContiguousFloat32Buffer).AsFloat32Buf()
	n := len(samples)
	o1 := o0 + fileOffset(n*r.bytesPerSample)
	leftover := 0
	if o1 > r.end {
		leftover = (int(o1-r.end) + r.bytesPerSample - 1) / r.bytesPerSample
		samples = samples[0 : n-leftover]
		o1 = r.end
	}
	nb := len(samples) * r.bytesPerSample
	if nb > len(r.buf) {
		r.buf = make([]byte, nb)
	}
	nr, err := io.ReadFull(r.fd, r.buf[0:nb])
	if err != nil {
		fmt.Fprintf(os.Stderr, "sample read error: %v\n", err)
		r.offset = r.end // ensure we don't try again
		leftover += (nb - nr + r.bytesPerFrame - 1) / r.bytesPerFrame
		samples = samples[0 : n-leftover]
		nb = nr
	}
	r.offset += fileOffset(nb)
	r.cvt(samples, r.buf)
	if leftover > 0 {
		samples = samples[len(samples):n]
		samples.Zero(0, len(samples))
		r.eof = true
	}
	return nb > 0
}

func makeWavWriter(status *abc.Status, args map[string]interface{}) Widget {
	defer un(log("makeWavWriter"))
	filename := args["2"].(string)
	fd, err := os.Create(filename)
	if fd == nil {
		panic("cannot open " + filename + ": " + err.String())
	}

	w := &WavWriter{}
	w.fd = fd
	w.Layout = Interleaved
	w.Type = Float32Type
	return w
}

func (w *WavWriter) Init(inputs map[string]Widget) {
	w.init(inputs["1"])
}

func (w *WavWriter) ReadSamples(_ Buffer, _ int64) bool {
	panic("readsamples called on output widget")
}

func WriteWav(filename string, input Widget) os.Error {
	fd, err := os.Create(filename)
	if fd == nil {
		return err
	}
	w := new(WavWriter)
	w.fd = fd
	w.init(input)
	return nil
}

func (w *WavWriter) init(input Widget) {
	endian := binary.LittleEndian
	const (
		bitsPerSample = 16
	)
	c0 := riffChunk{
		ChunkSize: 4 + // Format
			uint32(sizeof(fmtChunk{})) +
			uint32(sizeof(dataChunk{})), // excluding data length
		Format: str4("WAVE", endian),
	}
	format := input.GetFormat("out")
	c1 := fmtChunk{
		chunkHeader: chunkHeader{
			ChunkID:   str4("fmt ", endian),
			ChunkSize: uint32(sizeof(fmtChunk{})) - 8,
		},
		AudioFormat:   1,
		NumChannels:   int16(format.NumChans),
		SampleRate:    int32(format.Rate),
		ByteRate:      int32(format.Rate*format.NumChans) * (bitsPerSample / 8),
		BlockAlign:    int16(format.NumChans) * (bitsPerSample / 8),
		BitsPerSample: bitsPerSample,
	}
	c2 := dataChunk{
		ChunkID:   str4("data", endian),
		ChunkSize: 0, // excluding data length
	}
	w.fd.Write([]byte("RIFF"))
	binary.Write(w.fd, endian, c0)
	binary.Write(w.fd, endian, c1)
	binary.Write(w.fd, endian, c2)

	samples := AllocNFloat32Buf(int(c1.NumChannels), 8192)
	buf := make([]byte, samples.Size*int(c1.BlockAlign))
	p := int64(0)
	for input.ReadSamples(samples, p) {
		float32toint16le(buf, samples.Buf)
		w.fd.Write(buf)
		p += int64(samples.Size)
	}
	size := fileOffset(p) * fileOffset(c1.BlockAlign)
	if size > 0x7fffffff-fileOffset(c0.ChunkSize) {
		fmt.Fprintf(os.Stderr, "wav file limit exceeded")
		size = 0x7fffffff - fileOffset(c0.ChunkSize)
	}

	// rewrite header with correct size information
	c0.ChunkSize += uint32(size)
	c2.ChunkSize = uint32(size)
	w.fd.Seek(0, 0)
	w.fd.Write([]byte("RIFF"))
	binary.Write(w.fd, endian, c0)
	binary.Write(w.fd, endian, c1)
	binary.Write(w.fd, endian, c2)
}

func int16tofloat32le(samples []float32, data []byte) {
	j := 0
	for i := range samples {
		n := int16(data[j]) + int16(data[j+1])<<8
		samples[i] = float32(n) / 0x7fff
		j += 2
	}
}

func int16tofloat32be(samples []float32, data []byte) {
	j := 0
	for i := range samples {
		n := int16(data[j+1]) + int16(data[j])<<8
		samples[i] = float32(n) / 0x7fff
		j += 2
	}
}

func float32toint16le(data []byte, samples []float32) {
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
		n := int(s)
		data[j] = byte(n)
		data[j+1] = byte(n >> 8)
		j += 2
	}
}

func str4(s string, e binary.ByteOrder) (x uint32) {
	binary.Read(strings.NewReader(s), e, &x)
	return
}

func binread(r io.Reader, e binary.ByteOrder, x interface{}) {
	err := binary.Read(r, e, x)
	if err != nil {
		panic(Rerror(fmt.Sprintf("error reading binary: %v", err)))
	}
}
