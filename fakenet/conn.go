// The fakenet package provides a way to turn a regular io.ReadWriter
// into a net.Conn, including support for timeouts.
package fakenet

import (
	"io"
	"net"
	"time"
)

type Addr string

func (a Addr) Network() string {
	return "fakenet"
}

func (a Addr) String() string {
	return "fakenet:" + string(a)
}

// we've got a clash here between an underlying ReadWriteCloser that
// allows close to be called concurrently with Read or Write,
// (e.g. io.Pipe, ChanReader) and one that does not (most other
// ReadWriters).
type fakeConn struct {
	local  net.Addr
	remote net.Addr
	r      io.ReadCloser
	w      io.WriteCloser
}

// NewConn returns a net.Conn using r for reading and w for writing.
// 
// Local and remote give the addresses that will be returned
// from the respective methods of the connection.  If either is nil,
// Addr("fakenet") will be used.
func NewConn(r io.ReadCloser, w io.WriteCloser, local, remote net.Addr) net.Conn {
	// TODO:
	// If r implements the net.Conn.SetReadDeadline method,
	// or w implements the net.Conn.SetWriteDeadline method,
	// they will be used as appropriate.
	if local == nil {
		local = Addr("fakenet")
	}
	if remote == nil {
		remote = Addr("fakenet")
	}
	return &fakeConn{
		local:  local,
		remote: remote,
		r:      r,
		w:      w,
	}
}

func (c *fakeConn) LocalAddr() net.Addr {
	return c.local
}

func (c *fakeConn) RemoteAddr() net.Addr {
	return c.remote
}

func (c *fakeConn) Read(buf []byte) (n int, err error) {
	return c.r.Read(buf)
}

func (c *fakeConn) Write(buf []byte) (int, error) {
	return c.w.Write(buf)
}

func (c *fakeConn) Close() error {
	err := c.r.Close()
	// make sure we don't close the same thing twice.
	if err != nil || interface{}(c.w) == interface{}(c.r) {
		return err
	}
	return c.w.Close()
}

func (c *fakeConn) SetDeadline(t time.Time) error {
	return errUnimplemented
}

func (c *fakeConn) SetWriteDeadline(t time.Time) error {
	return errUnimplemented
}

func (c *fakeConn) SetReadDeadline(t time.Time) error {
	return errUnimplemented
}
