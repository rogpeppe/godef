package loopback

import (
	"errors"
	"io"
	"net"
	"time"
)

// NetPipe creates a synchronous, in-memory, full duplex
// network connection; both ends implement the net.Conn interface.
// The opt0 options apply to the traffic from c0 to c1;
// the opt1 options apply to the traffic from c1 to c0.
func NetPipe(opt0, opt1 Options) (c0 net.Conn, c1 net.Conn) {
	r0, w1 := Pipe(opt0)
	r1, w0 := Pipe(opt1)

	return &pipe{r0, w0}, &pipe{r1, w1}
}

type pipe struct {
	io.ReadCloser
	io.WriteCloser
}

type pipeAddr int

func (pipeAddr) Network() string {
	return "pipe"
}

func (pipeAddr) String() string {
	return "pipe"
}

func (p *pipe) Close() error {
	err := p.ReadCloser.Close()
	err1 := p.WriteCloser.Close()
	if err == nil {
		err = err1
	}
	return err
}

func (p *pipe) LocalAddr() net.Addr {
	return pipeAddr(0)
}

func (p *pipe) RemoteAddr() net.Addr {
	return pipeAddr(0)
}

func (p *pipe) SetDeadline(t time.Time) error {
	return errors.New("net.Pipe does not support deadlines")
}

func (p *pipe) SetReadDeadline(t time.Time) error {
	return errors.New("net.Pipe does not support deadlines")
}

func (p *pipe) SetWriteDeadline(t time.Time) error {
	return errors.New("net.Pipe does not support deadlines")
}
