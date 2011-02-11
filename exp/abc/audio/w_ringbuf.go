package audio

import (
	"fmt"
	"sync"
)

type RingBufWidget struct {
	minr0   int64 // minimum offset of any reader, in elements (but no reader can be more than maxsize behind r1, so r1 - minr0 <= maxsize)
	r1      int64 // offset of max sample in buffer + 1.
	samples Buffer
	size    int // current capacity of buffer (size < samples.Len()))
	readers []*RingBufReader
	maxsize int // maximum capacity.
	delta   int // rotational offset, to make resizing easier.

	lock    sync.Mutex
	missed  int64
	srcwait bool // writering can block?
	input	Widget	// get samples from here if non-nil
	reading bool
	readBufSize int
	readBuf Buffer
	fmt Format

	closed     bool      // writing process has gone away for good.
	waitc      chan bool // writing process wishes to be woken up.
	srcwaiting bool
	dstwaiting bool // at least one reading process wishes to be woken up

	srcp1 int64 // dst waiting to be able to write up to this point
}

type RingBufReader struct {
	Lag     int   // time delta (i.e. latency); zero implies primary output
	Blocking    bool  // Read can block for this reader?
	r0      int64 // read time.
	b       *RingBufWidget
	missed  int64
	waitc   chan bool // blocked awaiting data when non-nil.
	waiting bool
	p1      int64 // waiting for data up to here.
	index   int   // index into RingBuf readers of this reader.
}

type WritableRingBufWidget struct {
	RingBufWidget
}

func RingBuf(input Widget, size, maxsize int, readBufSize int) *RingBufWidget {
	if readBufSize < 0 {
		readBufSize = 0
	}
	var fmt Format
	if input != nil {
		fmt = input.GetFormat("out")
	}
	return new(RingBufWidget).init(size, maxsize, false, input, readBufSize, fmt)
}

func WriteableRingBuf(size, maxsize int, wblocking bool, fmt Format) *WritableRingBufWidget {
	var b WritableRingBufWidget
	b.init(size, maxsize, wblocking, nil, 0, fmt)
	return &b
}

func (b *RingBufWidget) Init(inputs map[string]Widget) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.input != nil {
		panic("cannot set input twice")
	}
	b.input = inputs["1"]
	b.setFormat(b.input.GetFormat("out"))
}

func (b *RingBufWidget) ReadSamples(_ Buffer, _ int64) bool {
	panic("ReadSamples on buffer head unit")
}

func (b *RingBufWidget) init(size, maxsize int,
		wblocking bool,
		input Widget,
		readBufSize int,
		fmt Format) *RingBufWidget {

	if readBufSize > size {
		panic("readBufSize > size")
	}
	b.fmt = fmt
	b.readers = make([]*RingBufReader, 0, 2)
	b.srcwait = wblocking
	b.input = input
	b.waitc = make(chan bool, 1)
	b.SetSizes(size, maxsize, readBufSize)
	return b
}

func (b *RingBufWidget) SetSizes(size, maxsize, readBufSize int) {
	b.size = size
	b.maxsize = maxsize
	// we allocate extra space at the end of the samples buffer, so that
	// we can always read directly into the buffer, even when it wraps
	if b.fmt.FullySpecified() {
		b.samples = b.fmt.AllocBuffer(b.size + readBufSize)
	}
	b.readBufSize = readBufSize
}

func (b *RingBufWidget) GetFormat(name string) Format {
	if name == "1" {
		return b.fmt
	}
	panic("unknown socket name")
}

func (b *RingBufWidget) Reader(blocking bool, lag int) *RingBufReader {
	b.lock.Lock()
	defer b.lock.Unlock()
	nr := len(b.readers)
	if nr >= cap(b.readers) {
		newr := make([]*RingBufReader, nr, nr*2)
		copy(newr, b.readers)
		b.readers = newr
	}
	r := &RingBufReader{
		Lag: lag,
		Blocking: blocking,
		r0: b.minr0,
		b: b,
		index: nr,
		waitc: make(chan bool, 1),
	}
	b.readers = b.readers[0 : nr+1]
	b.readers[nr] = r
	return r
}

func (b *RingBufWidget) SetFormat(fmt Format) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.input != nil {
		panic("too late to set format")
	}
	b.setFormat(fmt)
}

// internal form of SetFormat - called with lock held.
//
func (b *RingBufWidget) setFormat(fmt Format) {
	// throw away buffer - in the future, it might be possible
	// to do some kind of conversion.
	b.fmt = fmt
	oldlen := b.size + b.readBufSize
	if b.samples != nil {
		oldlen = b.samples.Len()
	}
	b.samples = fmt.AllocBuffer(oldlen)
	b.setminr0(b.r1)
	b.awakeWriter()
}

func (b *RingBufWidget) SetMaxSize(bufsize int) {
	b.lock.Lock()
	defer b.lock.Unlock()
	if b.maxsize < bufsize {
		b.maxsize = bufsize
	}
}

func (b *RingBufWidget) setminr0(minr0 int64) {
	b.minr0 = minr0
	for _, r := range b.readers {
		if r.r0 < minr0 {
			r.r0 = minr0
		}
	}
}

func (r *RingBufReader) Init(_ map[string]Widget) {
}

func (r *RingBufReader) setr0(r0 int64) {
	r.r0 = r0
	for _, r := range r.b.readers {
		if r.r0 < r0 {
			r0 = r.r0
		}
	}
	r.b.minr0 = r0
}

func (b *RingBufWidget) canWrite(p1 int64) bool {
	return p1-b.minr0 < int64(b.maxsize)
}

func (b *RingBufWidget) canRead(p1 int64) bool {
	return p1 <= b.r1 || b.closed
}

func (b *RingBufWidget) wakeReaders() {
	if !b.dstwaiting {
		return
	}
	stillwaiting := false
	for _, r := range b.readers {
		if r.waiting {
			if b.canRead(r.p1) {
				r.waitc <- true
				r.waiting = false
			} else {
				stillwaiting = true
			}
		}
	}
	b.dstwaiting = stillwaiting
}

func (b *RingBufWidget) Close() {
	b.lock.Lock()
	defer b.lock.Unlock()
	if !b.closed {
		b.closed = true
		b.wakeReaders()
	}
}

func (r *RingBufReader) Close() {
	// XXX
}

func (b *WritableRingBufWidget) SampleWrite(samples Buffer, p0 int64) {
	if samples.Len() > b.maxsize {
		panic("single write of more than maximum size of buffer")
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	p1 := p0 + int64(samples.Len())
	if b.srcwait {
		b.waitForSpace(p1)
	}
	b.write(samples, p0)
	b.wakeReaders()
}

// wait until there's space to write up to p1.
// called with lock held.
func (b *RingBufWidget) waitForSpace(p1 int64) {
	if !b.canWrite(p1) {
debugp("no space (size %v; p1 %v; minr0 %v; r1 %v)\n", b.size, p1, b.minr0, b.r1)
		b.srcwaiting = true
		b.srcp1 = p1
		b.lock.Unlock()
		<-b.waitc
		b.lock.Lock()
	}
}

func (r *RingBufReader) ReadSamples(samples Buffer, p0 int64) bool {
	b := r.b
	b.lock.Lock()
defer un(log("buf read %v [%v] (minr0 %v, r1 %v, delta %v, size %v)", p0, samples.Len(), b.minr0, b.r1, b.delta, b.size))
	defer b.lock.Unlock()
	p0 -= int64(r.Lag)
	p1 := p0 + int64(samples.Len())
	if r.Blocking && !b.canRead(p1) {
		if b.input == nil || b.reading {
			b.dstwaiting = true
			r.waiting = true
			r.p1 = p1
			b.lock.Unlock()
			<-r.waitc
			b.lock.Lock()
		}else{
			// keep reading at end of buffer until we can satisfy the read.
			b.reading = true
			readsize := b.readBufSize
			if readsize <= 0 {
				n := samples.Len()
				if b.size + n > b.samples.Len() {
					nb := b.fmt.AllocBuffer(b.size + n)
					nb.Copy(0, b.samples, 0, b.size)
					b.samples = nb
				}
				readsize = n
			}
			if readsize > b.maxsize {
				readsize = b.maxsize
			}
			for {
				wp1 := b.r1 + int64(readsize)
				b.waitForSpace(wp1)
				if wp1 - b.minr0 > int64(b.size) {
					b.growBuf(b.r1, wp1)
				}

				off := int((b.r1 + int64(b.delta)) % int64(b.size))
				buf := b.samples.Slice(off, off + readsize)

				b.lock.Unlock()
				ok := b.input.ReadSamples(buf, b.r1)
				if ok {
					// if it wraps, copy the overhanging tail to the start of the buffer.

					if off + readsize > b.size {
						b.samples.Copy(0, buf, b.size - off, readsize)
					}
				}

				b.lock.Lock()
				if ok {
					b.r1 = wp1
				}else{
					b.closed = true
				}
				b.wakeReaders()
				if b.canRead(p1) {
					break
				}
			}
			b.reading = false
		}
	}
	// allow final samples to be read, even if we finish with zeros.
	if b.closed && p0 >= b.r1 {
		return false
	}
	r.read(samples, p0)
	b.awakeWriter()
	return true
}

func (b *RingBufWidget) awakeWriter() {
	if b.srcwaiting && b.canWrite(b.srcp1) {
		b.waitc <- true
		b.srcwaiting = false
	}
}

func (b *RingBufWidget) writeMissed(n int64) {
	fmt.Printf("write missed %v\n", n)
	b.missed += n
}

func (r *RingBufReader) readMissed(n int64, why string) {
	fmt.Printf("read missed %v\n", n)
	if n < 10 {
		panic(fmt.Sprintf("read skip %d (%s)\n", n, why))
	}
	r.missed += n
}


// growBuf grows the buffer to ensure that
// there is space to write the samples [p0, p1]
// (p1 - p0 <= b.maxsize)
func (b *RingBufWidget) growBuf(p0, p1 int64) {
debugp("growbuf (old %v) %v %v\n", b.size, p0, p1)

	// if buffer is empty, don't bother expanding unless
	// we need to.
	if b.r1 == b.minr0 {
		b.setminr0(p0)
		b.r1 = p0
	}
	nsize := p1 - b.minr0
	if nsize > int64(b.maxsize) {
		nsize = int64(b.maxsize)
		// lose any old values that won't fit.
		if p1-nsize >= b.r1 {
			// losing (b->r1 - b->minr0) values
			b.setminr0(p0)
			b.r1 = p0
			if p1-p0 < int64(b.size) {
				// no need to grow
				return
			}
		} else {
			// losing (p1 - nsize - b.minr0) values, but keeping some
			b.setminr0(p1 - nsize)
		}
	}

	// keep same amount of extra space at end of buffer
	nbuf := b.fmt.AllocBuffer(int(nsize) + (b.samples.Len() - b.size))
	otail := b.size - int((b.minr0+int64(b.delta)+int64(b.size))%int64(b.size))
	ntail := int(nsize) - int(b.minr0%nsize)
	len := int(p1 - b.minr0)
	oldoff := b.size - otail
	if otail-len >= 0 {
		nbuf.Copy(int(nsize)-otail, b.samples, oldoff, oldoff+len)
	} else {
		nbuf.Copy(int(nsize)-otail, b.samples, oldoff, oldoff+otail)
		nbuf.Copy(0, b.samples, 0, len-otail)
	}
	b.samples = nbuf
	b.delta = ntail - otail
	b.size = int(nsize)
}

func (b *RingBufWidget) write(samples Buffer, p0 int64) {
	n := samples.Len()
	p1 := p0 + int64(n)
	if p0 < b.r1 {
		panic("write in the past")
	}
	if p1-b.minr0 > int64(b.size) && b.size < b.maxsize {
		b.growBuf(p0, p1)
	}
	off := int((p0 + int64(b.delta)) % int64(b.size))
	if p0 != b.r1 {
		// write beyond the end of the buffer - adjust buffer
		// so that it ends at p0, keeping as much data as possible.
		b.writeMissed(p0 - b.r1)

		if p0 >= b.r1+int64(b.size) || b.minr0 == b.r1 {
			// none of the original buffer remains; just copy in new buffer
			b.setminr0(p0)
		} else {
			// there's some missing data - fill in gap with zeros
			gap := int(p0 - b.r1)
			i := off - gap
			if i < 0 {
				b.samples.Zero(b.size, b.size-i)
				b.samples.Zero(0, off)
			} else {
				b.samples.Zero(i, i+gap)
			}

			r0 := p1 - int64(b.size)
			if r0 > b.minr0 {
				b.setminr0(r0)
			}
		}
		b.r1 = p0
	}
	if off+n <= b.size {
		b.samples.Copy(off, samples, 0, n)
	} else {
		gap := b.size - off
		b.samples.Copy(off, samples, 0, gap)
		b.samples.Copy(0, samples, gap, n)
	}
	if p1-b.minr0 > int64(b.size) {
		b.setminr0(p1 - int64(b.size))
	}
	b.r1 = p1
}

func (r *RingBufReader) GetFormat(name string) Format {
	if name == "out" {
		return r.b.fmt
	}
	panic("unknown socket name")
}

func (r *RingBufReader) SetFormat(f Format) {
	// XXX lock r.b? remake/resize buffers?
	r.b.fmt = f
}

// read actually reads the values from the buffer.
// called with buffer lock held.
func (r *RingBufReader) read(samples Buffer, p0 int64) {
	b := r.b
	n := samples.Len()
	p1 := p0 + int64(n)
	off := int((p0 + int64(b.delta)) % int64(b.size))

debugp("actual read [%v %v] off %v\n", p0, p1, off)
	if n > b.maxsize {
		panic("invariant violated")
	}

	gap := b.minr0 - p0
	if gap > 0 {
		// read too early for buffer - zero-fill start of samples
		if gap >= int64(n) {
			samples.Zero(0, n)
			r.r0 = b.minr0			// XXX unnecessary?
			r.readMissed(int64(n), "much too slow")
			return
		}
		g := int(gap)
		samples.Zero(0, g)
		samples = samples.Slice(g, n)
		n -= g
		p0 += gap
		r.readMissed(gap, "too slow")
	}

	gap = p1 - b.r1
	if gap > 0 {
		// read after end of buffer - zero-fill end of samples
		if gap >= int64(n) {
			samples.Zero(0, n)
			r.setr0(b.r1)
			r.readMissed(int64(n), "much too fast")
			return
		}
		g := int(gap)
		samples.Zero(n-g, n)
		n -= g
		p1 -= gap
		p0 += gap
		if !b.closed {
			r.readMissed(gap, "too fast")
		}
	}

	if off+n <= b.size {
		samples.Copy(0, b.samples, off, off+n)
	} else {
		gap := b.size - off
		samples.Copy(0, b.samples, off, off+gap)
		samples.Copy(gap, b.samples, 0, n-gap)
	}
	r.setr0(p1)
}
