package fakenet

import (
	"io"
	"net"
)

type listener struct {
	addr     net.Addr
	closedCh chan bool
	conns    chan net.Conn
}

// NewListener creates a new net.Listener and returns a channel on which
// it reads connections and gives to callers of Accept.  The Listener
// returns addr for its address (or Addr("fakenet") if nil).
func NewListener(addr net.Addr) (chan<- net.Conn, net.Listener) {
	if addr == nil {
		addr = Addr("fakenet")
	}
	l := &listener{addr, make(chan bool), make(chan net.Conn)}
	return l.conns, l
}

func (l *listener) Close() error {
	close(l.closedCh)
	return nil
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case c, ok := <-l.conns:
		if !ok {
			return nil, io.EOF
		}
		return c, nil
	case <-l.closedCh:
	}
	return nil, ErrClosed
}

func (l *listener) Addr() net.Addr {
	return l.addr
}
