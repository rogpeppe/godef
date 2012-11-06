package fakenet

import (
	"net"
	"testing"
	"time"
)

type pipeConn struct {
	c net.Conn
	r <-chan []byte
	w chan<- []byte
}

func newPipeConn() *pipeConn {
	c0 := make(chan []byte)
	c1 := make(chan []byte)
	c := &pipeConn{
		c: NewConn(NewChanReader(c0), NewChanWriter(c1), nil, nil),
		r: c1,
		w: c0,
	}
	// sleep for a little while to let the pipe
	// initialisation settle down so it doesn't
	// skew the timing tests.
	time.Sleep(0.05e9)
	return c
}

// pre Go-1 tests, awaiting SetDeadline
//func TestReadTimeout(t *testing.T) {
//	p := newPipeConn()
//	const timeout = int64(0.1e9)
//	if err := p.c.SetReadTimeout(timeout); err != nil {
//		t.Fatalf("error setting read timeout: %v", err)
//	}
//	buf := make([]byte, 10)
//	t0 := time.Now()
//	n, err := p.c.Read(buf)
//	t1 := time.Now()
//	elapsed := t1.Sub(t0)
//	if n > 0 || err != os.ETIMEDOUT {
//		t.Fatalf("expected timeout, got n=%d, err=%v\n", n, err)
//	}
//	if abs(elapsed-timeout)*100/timeout > 1 {
//		t.Errorf("timeout time expected %d; got %d\n", timeout, elapsed)
//	}
//}
//
//func TestReadAfterTimeout(t *testing.T) {
//	p := newPipeConn()
//	const timeout = int64(0.1e9)
//	if err := p.c.SetReadTimeout(timeout); err != nil {
//		t.Fatalf("error setting read timeout: %v", err)
//	}
//
//	// check that a read after timeout does not get lost
//	go func() {
//		time.Sleep(timeout + timeout/2)
//		p.w <- []byte("hello")
//	}()
//	buf := make([]byte, 10)
//	n, err := p.c.Read(buf)
//	if n > 0 || err != os.ETIMEDOUT {
//		t.Fatalf("expected timeout, got n=%d, err=%v\n", n, err)
//	}
//	time.Sleep(timeout) // wait until write has occurred
//	n, err = p.c.Read(buf)
//	if err != nil {
//		t.Fatalf("read error: %v", err)
//	}
//	if n != 5 || string(buf[0:n]) != "hello" {
//		t.Fatalf("got wrong data; expected %q; got %q", "hello", buf[0:n])
//	}
//}

func TestCloseWhileReading(t *testing.T) {
	p := newPipeConn()
	c := make(chan bool)
	go func() {
		n, err := p.c.Read(make([]byte, 10))
		if n != 0 || err != ErrClosed {
			t.Fatalf("read returned wrongly; expected 0, EINVAL; got %d, %v", n, err)
		}
		c <- true
	}()
	select {
	case <-c:
		t.Fatalf("read did not block")
	case <-time.After(0.2e9):
	}
	p.c.Close()
	select {
	case <-time.After(0.2e9):
		t.Fatalf("read still blocked after close")
	case <-c:
	}
}

func abs(x int64) int64 {
	if x >= 0 {
		return x
	}
	return -x
}
