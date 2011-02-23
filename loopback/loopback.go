package loopback

import (
	"sync"
	"io"
	"os"
	"time"
)

type block struct {
	t    int64
	data []byte
	prev *block
	next *block
}

type streamReader stream
type streamWriter stream

func (r *streamReader) Read(data []byte) (int, os.Error) {
	return (*stream)(r).Read(data)
}

func (r *streamReader) Close() os.Error {
	return (*stream)(r).closeInput()
}

func (w *streamWriter) Write(data []byte) (int, os.Error) {
	return (*stream)(w).Write(data)
}

func (w *streamWriter) Close() os.Error {
	return (*stream)(w).closeOutput()
}

type stream struct {
	mu sync.Mutex

	outClosed bool
	inClosed  bool

	outTail     *block // sentinel.
	outHead     *block // also transitTail.
	transitHead *block // also inTail.
	inHead      *block // overall head of list.

	outLimit int // total size of output queue.
	outAvail int // free bytes in output queue.

	inLimit int // total size of input queue.
	inAvail int // free bytes in input queue.

	byteDelay int64
	latency   int64
	mtu       int

	notEmpty     sync.Cond
	notFull      sync.Cond
}

// Loopback options for use with Pipe.
type Options struct {
	// ByteDelay controls the time a packet takes in the link.  A packet
	// n bytes long takes ByteDelay * n nanoseconds to exit
	// the output queue and is available for reading Latency
	// nanoseconds later.
	ByteDelay int64
	Latency   int64

	// MTU gives the maximum packet size that can
	// be tranferred atomically across the link.
	// Larger packet will be split.
	// If this is zero, a default of 32768 is assumed
	MTU int

	// InLimit and OutLimit gives the size of the input and output queues.
	// If either is zero, a default of 10*MTU is assumed.
	InLimit  int
	OutLimit int
}

// Pipe creates an asynchronous in-memory pipe,
// Writes are divided into packets of at most opts.MTU bytes
// written to a flow-controlled output queue, transferred across the link,
// and put into an input queue where it is readable with the r.
// The options determine when and how the data will be transferred.
func Pipe(opt Options) (r io.ReadCloser, w io.WriteCloser) {
	if opt.MTU == 0 {
		opt.MTU = 32768
	}
	if opt.InLimit == 0 {
		opt.InLimit = 10 * opt.MTU
	}
	if opt.OutLimit == 0 {
		opt.OutLimit = 10 * opt.MTU
	}
	if opt.InLimit < opt.MTU {
		opt.InLimit = opt.MTU
	}
	if opt.OutLimit < opt.MTU {
		opt.OutLimit = opt.MTU
	}
	sentinel := &block{}
	s := &stream{
		outLimit:    opt.OutLimit,
		outAvail:    opt.OutLimit,
		inLimit:     opt.InLimit,
		inAvail:     opt.InLimit,
		mtu:         opt.MTU,
		byteDelay:   opt.ByteDelay,
		latency:     opt.Latency,
		outTail:     sentinel,
		outHead:     sentinel,
		transitHead: sentinel,
		inHead:      sentinel,
	}
	s.notEmpty.L = &s.mu
	s.notFull.L = &s.mu
	return (*streamReader)(s), (*streamWriter)(s)
}

// Dodgy heuristic:
// If there's stuff in the transit queue that's ready to
// enter the input queue, but the input queue is full
// and it's been waiting for at least latency ns,
// then we block the output queue.
// TODO what do we do about latency for
// blocked packets - as it is a blocked packet
// will incur less latency.
func (s *stream) outBlocked(now int64) bool {
	return s.transitHead != s.outHead &&
		now >= s.transitHead.t+s.latency &&
		s.inAvail < len(s.transitHead.data)
}

func (s *stream) closeInput() os.Error {
	s.mu.Lock()
	s.inClosed = true
	s.notEmpty.Broadcast()
	s.notFull.Broadcast()
	s.mu.Unlock()
	return nil
}

func (s *stream) closeOutput() os.Error {
	s.mu.Lock()
	s.outClosed = true
	s.notEmpty.Broadcast()
	s.notFull.Broadcast()
	s.mu.Unlock()
	return nil
}

func (s *stream) pushLink(now int64) {
	if !s.outBlocked(now) {
		// move blocks from out queue to transit queue.
		for s.outTail != s.outHead && now >= s.outHead.t {
			s.outHead.t += s.latency
			s.outAvail += len(s.outHead.data)
			s.outHead = s.outHead.next
		}
	}
	// move blocks from transit queue to input queue
	for s.transitHead != s.outHead && now >= s.transitHead.t {
		if s.inAvail < len(s.transitHead.data) {
			break // or discard packet
		}
		s.inAvail -= len(s.transitHead.data)
		s.transitHead = s.transitHead.next
	}
}

func (s *stream) Write(data []byte) (int, os.Error) {
	// split the packet into MTU-sized portions if necessary.
	for len(data) > s.mtu {
		_, err := s.Write(data[0:s.mtu])
		if err != nil {
			return 0, err
		}
		data = data[s.mtu:]
	}
	s.mu.Lock()
	now := time.Nanoseconds()
	for {
		s.pushLink(now)
		if s.outAvail >= len(data) || s.outClosed {
			break
		}
		if s.outBlocked(time.Nanoseconds()) {
			if s.inClosed {
				s.mu.Unlock()
				return 0, os.EPIPE
			}
			s.notFull.Wait()
			continue
		}
		t := s.earliestWriteTime(len(data))
		now = s.sleepUntil(t)
	}
	if s.outClosed {
		s.mu.Unlock()
		return 0, os.EPIPE
	}
	delay := int64(len(data)) * s.byteDelay
	var t int64
	// If there's a block in the queue that's not yet due
	// for transit, then this block leaves delay ns after
	// that one.
	if s.outHead != s.outTail && now < s.outTail.prev.t {
		t = s.outTail.prev.t + delay
	} else {
		t = now + delay
	}
	s.addBlock(t, s.copy(data))
	s.outAvail -= len(data)

	s.notEmpty.Broadcast()
	s.mu.Unlock()
	// TODO runtime.Gosched() ?
	return len(data), nil
}

func (s *stream) Read(buf []byte) (int, os.Error) {
	s.mu.Lock()
	// Loop until there's something to read from the input queue.
	now := time.Nanoseconds()
	for {
		s.pushLink(now)
		if s.inHead != s.transitHead {
			break
		}
		if s.inHead == s.outTail {
			// No data at all in the queue.
			// If the queue is empty and the output queue is closed,
			// then we see EOF.
			if s.outClosed {
				s.mu.Unlock()
				return 0, os.EOF
			}
			s.notEmpty.Wait()
			continue
		}
		now = s.sleepUntil(s.earliestReadTime())
	}
	if s.inClosed {
		// input queue has been forcibly closed:
		// TODO is os.EOF the right error here?
		return 0, os.EOF
	}
	b := s.inHead
	n := copy(buf, b.data)
	b.data = b.data[n:]
	s.inAvail += n
	if len(b.data) == 0 {
		s.removeBlock()
	}
	// Wake up any writers blocked on a full queue.
	s.notFull.Broadcast()
	s.mu.Unlock()
	return n, nil
}

// earliestReadTime returns the earliest time that
// some data might arrive into the input queue.
// It assumes that there is some data in the system.
func (s *stream) earliestReadTime() int64 {
	if s.inAvail < s.inLimit {
		// data is available right now.
		return 0
	}
	if s.transitHead != s.outHead {
		return s.transitHead.t
	}
	if s.outHead != s.outTail {
		return s.outHead.t + s.latency
	}
	panic("no data")
}

// earliestWriteTime returns the earliest time that
// there may be space for n bytes of data to be
// placed into the output queue (it might be later
// if packets are dropped).
func (s *stream) earliestWriteTime(n int) int64 {
	if s.outAvail < s.outLimit {
		// space is available now.
		return 0
	}
	tot := s.outAvail
	for b := s.outHead; b != s.outTail; b = b.next {
		tot += len(b.data)
		if tot >= n {
			return b.t
		}
	}
	panic("write limit exceeded by block size")
}

// sleep until the absolute time t.
// Called with lock held.
func (s *stream) sleepUntil(t int64) int64 {
	now := time.Nanoseconds()
	if now >= t {
		return now
	}
	s.mu.Unlock()
	time.Sleep(t - now)
	s.mu.Lock()
	return time.Nanoseconds()
}

func (s *stream) copy(x []byte) []byte {
	y := make([]byte, len(x))
	copy(y, x)
	return y
}

// addBlock adds a block to the head of the queue.
// It does not adjust queue stats.
func (s *stream) addBlock(t int64, data []byte) {
	// If there are no items in output queue, replace sentinel block
	// so that other pointers into queue do not need
	// to change.
	if s.outHead == s.outTail {
		s.outHead.t = t
		s.outHead.data = data
		s.outHead.next = &block{prev: s.outHead} // new sentinel
		s.outTail = s.outHead.next
		return
	}

	// Add a new block just after the sentinel.	
	b := &block{
		t:    t,
		data: data,
	}
	b.next = s.outTail
	b.prev = s.outTail.prev

	s.outTail.prev = b
	b.prev.next = b
}

// Remove the block from the front of the queue.
// (assumes that there is such a block to remove)
func (s *stream) removeBlock() {
	b := s.inHead
	s.inHead = b.next
	if s.inHead != nil {
		s.inHead.prev = nil
	}
	// help garbage collector
	b.next = nil
	b.prev = nil
}
