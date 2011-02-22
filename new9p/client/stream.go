package client

import (
	"io"
	"os"
	"sync"
	"rog-go.googlecode.com/hg/new9p/seq"
"log"
	"container/list"
)

type readResult struct {
	buf []byte
	err os.Error
}

type streamReader struct {
	c     chan readResult
	reply chan bool
	buf   []byte
	done  bool
}

func (cr *streamReader) Read(buf []byte) (int, os.Error) {
	if cr.done {
		return 0, os.EOF
	}
	if len(cr.buf) == 0 {
		// no bytes in buffer: try to get some more.
		r := <-cr.c
		if len(r.buf) == 0 {
			// stream has come to an end.
			close(cr.reply)
			cr.done = true
			return 0, r.err
		}
		cr.buf = r.buf
	}
	// send some bytes that we've already got.
	n := copy(buf, cr.buf)
	cr.buf = cr.buf[n:]
	if len(cr.buf) == 0 {
		cr.reply <- true
	}
	return n, nil
}

func (cr *streamReader) Close() os.Error {
//	cr.seq.Flush()
	if !cr.done {
		close(cr.reply)
		cr.done = true
	}
	return nil
}

func (cr *streamReader) WriteTo(w io.Writer) (tot int64, err os.Error) {
	if cr.done {
		return 0, os.EOF
	}
	if len(cr.buf) > 0 {
		n, err := w.Write(cr.buf)
		tot += int64(n)
		if err != nil {
			return tot, err
		}
	}
	for {
		r := <-cr.c
		if r.err != nil {
			cr.done = true
			if r.err == os.EOF {
				r.err = nil
			}
			return tot, r.err
		}
		n, err := w.Write(r.buf)
		tot += int64(n)
		if n < len(r.buf) {
			cr.reply <- false
			return tot, err
		}
		cr.reply <- true
	}
	return
}

func (nsf *NsFile) SeqReadStream(sq *seq.Sequencer, nreqs, iounit int) io.ReadCloser {
	cr := &streamReader{
		c:     make(chan readResult, 1),
		reply: make(chan bool),
	}
	sq, results := sq.Subsequencer("stream reader")
	buf := make([]byte, nreqs*iounit)
	bufs := make(chan []byte, nreqs)
	for i := 0; i < nreqs; i++ {
		bufs <- buf[0:iounit]
		buf = buf[iounit:]
	}
	buf = nil
	var q safeQueue
	done := make(chan bool)

	// Stream requests.
	go func() {
		f := nsf.File()
		offset := int64(0)
		for {
			b := <-bufs
			if closed(bufs) {
				break
			}
			q.Put(b)
log.Printf("stream doer: read %v", offset)
			sq.Do(f, seq.ReadReq{b, offset})
			offset += int64(len(b))
		}
log.Printf("stream doer: do(nil, nil)")
		sq.Do(nil, nil)
		done <- true
	}()

	// Stream replies on demand from the streamReader.
	go func() {
		readerClosed := false
		for r := range results {
log.Printf("stream: got result %#v (chan %p)\n", r, results)
			b := q.Get().([]byte)
			cr.c <- readResult{b[0:r.(seq.ReadResult).Count], nil}
			if !<-cr.reply {
				readerClosed = true
				break
			}
			bufs <- b
		}
log.Printf("stream: closed")
		// Stop as many requests as possible from being sent.
		// If we implemented flush, we would flush the request now.
	loop:
		for {
			select{
			case <-bufs:
			default:
				break loop
			}
		}
		close(bufs)

		// Absorb and ignore any extra replies.
		for r := range results {
log.Printf("stream: aborbing extra: %#v\n", r)
		}
		<-done

		err := sq.Error()
		if !readerClosed {
log.Printf("stream: sending error to reader")
			if err == nil {
				err = os.EOF
			}
			cr.c <- readResult{nil, err}
		}
log.Printf("stream: yielding result, err %#v\n", err)
		sq.Result(seq.StringResult("SeqReadStream"), err)
	}()
	return cr
}

type safeQueue struct {
	mu sync.Mutex
	l  list.List
}

func (q *safeQueue) Put(x interface{}) {
	q.mu.Lock()
	q.l.PushBack(x)
	q.mu.Unlock()
}

func (q *safeQueue) Get() interface{} {
	q.mu.Lock()
	e := q.l.Front()
	q.l.Remove(e)
	q.mu.Unlock()
	return e.Value
}
