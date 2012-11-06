package fakenet

import (
	"errors"
	"io"
	"runtime"
	"sync"
	"time"
)

// A ChanReader reads from a chan []byte to
// satisfy Read requests.
type ChanReader struct {
	mu       sync.Mutex
	buf      []byte
	c        <-chan []byte
	closedCh chan bool
	closed   bool
}

// NewChanReader creates a new ChanReader that
// reads from the given channel.
func NewChanReader(c <-chan []byte) *ChanReader {
	return &ChanReader{c: c, closedCh: make(chan bool)}
}

// Close implements the net.Conn Close method.
func (r *ChanReader) Close() error {
	close(r.closedCh)
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}

var ErrClosed = errors.New("operation on closed channel")
var errUnimplemented = errors.New("unimplemented")

// SetReadDeadline implements the net.Conn SetReadDeadline method.
func (r *ChanReader) SetReadDeadline(t time.Time) error {
	return errUnimplemented
}

// Read implements the net.Conn Read method.
func (r *ChanReader) Read(buf []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrClosed
	}
	for len(r.buf) == 0 {
		var ok bool
		select {
		case r.buf, ok = <-r.c:
			if !ok {
				return 0, io.EOF
			}
		case <-r.closedCh:
			return 0, ErrClosed
		}
	}
	n := copy(buf, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

// A ChanWriter writes to a chan []byte to satisfy Write requests.
type ChanWriter struct {
	c      chan<- []byte
	closed chan bool
}

// NewChanWriter creates a new ChanWriter that writes
// to the given channel.
func NewChanWriter(c chan<- []byte) *ChanWriter {
	return &ChanWriter{c: c, closed: make(chan bool)}
}

// SetWriteDeadline implements the net.Conn SetWriteDeadline method.
func (w *ChanWriter) SetWriteDeadline(t time.Time) error {
	return errUnimplemented
}

const errChanClosed = "runtime error: send on closed channel"

var errWriteOnClosedPipe = errors.New("write on closed pipe")

// Write implements the net.Conn Write method.
func (w *ChanWriter) Write(buf []byte) (n int, err error) {
	// We catch the "send on closed channel" error because
	// there's an inherent race between Write and Close that's
	// allowed for a net.Conn but not allowed for channels.
	defer func() {
		if e := recover(); e != nil {
			if e, ok := e.(runtime.Error); ok && e.Error() == errChanClosed {
				n = 0
				err = errWriteOnClosedPipe
			} else {
				panic(e)
			}
		}
	}()
	b := make([]byte, len(buf))
	copy(b, buf)
	w.c <- b
	return len(buf), nil
}

// Close implements the net.Conn Close method.
func (w *ChanWriter) Close() error {
	close(w.c)
	return nil
}
