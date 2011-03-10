package loopback
import (
	"io"
	"os"
	"net"
	"rog-go.googlecode.com/hg/fakenet"
)

// Dial is the same as net.Dial except that it also recognises
// networks with the prefix "loopback:"; it removes
// the prefix, dials the original network, and then applies
// the given loopback Options. Incoming data has inOpts
// applied; outgoing data has outOpts applied.
func Dial(netw, laddr, raddr string) (net.Conn, os.Error) {
	if netw != "" && netw[0] != '[' {
		return net.Dial(netw, laddr, raddr)
	}
	inOpts, outOpts, actualNet, err := parseNetwork(netw)
	if err != nil {
		return nil, err
	}
	c, err := net.Dial(actualNet, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return NewConn(c, inOpts, outOpts), nil
}

func NewConn(c net.Conn, inOpts, outOpts Options) net.Conn {
	r0, w0 := Pipe(inOpts)
	r1, w1 := Pipe(outOpts)
	go func() {
		io.Copy(w1, c)
		w1.Close()
	}()

	go func() {
		io.Copy(c, r0)
		c.Close()
	}()
	return fakenet.NewConn(r1, w0, c.LocalAddr(), c.RemoteAddr())
}

type listener struct {
	inOpts, outOpts Options
	l net.Listener
}

// Dial is the same as net.Listen except that it also recognises
// networks with the [attr=val, attr=val, ...]network; it removes
// the prefix, listens on the original network, and then applies
// the given loopback Options to each connection. Incoming data has inOpts
// applied; outgoing data has outOpts applied.
func Listen(netw, laddr string) (net.Listener, os.Error) {
	if netw != "" && netw[0] != '[' {
		return net.Listen(netw, laddr)
	}
	inOpts, outOpts, actualNet, err := parseNetwork(netw)
	if err != nil {
		return nil, err
	}
	return ListenOpts(actualNet, laddr, inOpts, outOpts)
}

func ListenOpts(netw, laddr string, inOpts, outOpts Options) (net.Listener, os.Error) {
	l, err := net.Listen(netw, laddr)
	if err != nil {
		return nil, err
	}
	return &listener{inOpts, outOpts, l}, nil
}

func (l *listener) Addr() net.Addr {
	return l.l.Addr()
}

func (l *listener) Accept() (net.Conn, os.Error) {
	c, err := l.l.Accept()
	if err != nil {
		return nil, err
	}
	return NewConn(c, l.inOpts, l.outOpts), nil
}

func (l *listener) Close() os.Error {
	return l.l.Close()
}
