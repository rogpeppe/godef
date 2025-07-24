//go:build !plan9
// +build !plan9

package client // import "9fans.net/go/plan9/client"

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"9fans.net/go/plan9"
)

type Error string

func (e Error) Error() string { return string(e) }

type Conn struct {
	// We wrap the underlying conn type so that
	// there's a clear distinction between Close,
	// which forces a close of the underlying rwc,
	// and Release, which lets the Fids take control
	// of when the conn is actually closed.
	mu       sync.Mutex
	_c       *conn
	released bool
}

var errClosed = fmt.Errorf("connection has been closed")

// Close forces a close of the connection and all Fids derived
// from it.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c._c == nil {
		if c.released {
			return fmt.Errorf("cannot close connection after it's been released")
		}
		return nil
	}
	rwc := c._c.rwc
	c._c = nil
	// TODO perhaps we shouldn't hold the mutex while closing?
	return rwc.Close()
}

func (c *Conn) conn() (*conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c._c == nil {
		return nil, errClosed
	}
	return c._c, nil
}

// Release marks the connection so that it will
// close automatically when the last Fid derived
// from it is closed.
//
// If there are no current Fids, it closes immediately.
// After calling Release, c.Attach, c.Auth and c.Close will return
// an error.
func (c *Conn) Release() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c._c == nil {
		return nil
	}
	conn := c._c
	c._c = nil
	c.released = true
	return conn.release()
}

type conn struct {
	rwc      io.ReadWriteCloser
	err      error
	tagmap   map[uint16]chan *plan9.Fcall
	freetag  map[uint16]bool
	freefid  map[uint32]bool
	nexttag  uint16
	nextfid  uint32
	msize    uint32
	version  string
	w, x     sync.Mutex
	muxer    bool
	refCount int32 // atomic
}

func NewConn(rwc io.ReadWriteCloser) (*Conn, error) {
	c := &conn{
		rwc:      rwc,
		tagmap:   make(map[uint16]chan *plan9.Fcall),
		freetag:  make(map[uint16]bool),
		freefid:  make(map[uint32]bool),
		nexttag:  1,
		nextfid:  1,
		msize:    131072,
		version:  "9P2000",
		refCount: 1,
	}

	//	XXX raw messages, not c.rpc
	tx := &plan9.Fcall{Type: plan9.Tversion, Tag: plan9.NOTAG, Msize: c.msize, Version: c.version}
	err := c.write(tx)
	if err != nil {
		return nil, err
	}
	rx, err := c.read()
	if err != nil {
		return nil, err
	}
	if rx.Type != plan9.Rversion || rx.Tag != plan9.NOTAG {
		return nil, plan9.ProtocolError(fmt.Sprintf("invalid type/tag in Tversion exchange: %v %v", rx.Type, rx.Tag))
	}

	if rx.Msize > c.msize {
		return nil, plan9.ProtocolError(fmt.Sprintf("invalid msize %d in Rversion", rx.Msize))
	}
	c.msize = rx.Msize
	if rx.Version != "9P2000" {
		return nil, plan9.ProtocolError(fmt.Sprintf("invalid version %s in Rversion", rx.Version))
	}
	return &Conn{
		_c: c,
	}, nil
}

func (c *conn) newFid(fid uint32, qid plan9.Qid) *Fid {
	c.acquire()
	return &Fid{
		_c:  c,
		fid: fid,
		qid: qid,
	}
}

func (c *conn) newfidnum() (uint32, error) {
	c.x.Lock()
	defer c.x.Unlock()
	for fidnum := range c.freefid {
		delete(c.freefid, fidnum)
		return fidnum, nil
	}
	fidnum := c.nextfid
	if c.nextfid == plan9.NOFID {
		return 0, plan9.ProtocolError("out of fids")
	}
	c.nextfid++
	return fidnum, nil
}

func (c *conn) putfidnum(fid uint32) {
	c.x.Lock()
	defer c.x.Unlock()
	c.freefid[fid] = true
}

func (c *conn) newtag(ch chan *plan9.Fcall) (uint16, error) {
	c.x.Lock()
	defer c.x.Unlock()
	var tagnum uint16
	for tagnum = range c.freetag {
		delete(c.freetag, tagnum)
		goto found
	}
	tagnum = c.nexttag
	if c.nexttag == plan9.NOTAG {
		return 0, plan9.ProtocolError("out of tags")
	}
	c.nexttag++
found:
	c.tagmap[tagnum] = ch
	if !c.muxer {
		c.muxer = true
		ch <- &yourTurn
	}
	return tagnum, nil
}

func (c *conn) puttag(tag uint16) chan *plan9.Fcall {
	c.x.Lock()
	defer c.x.Unlock()
	ch := c.tagmap[tag]
	delete(c.tagmap, tag)
	c.freetag[tag] = true
	return ch
}

func (c *conn) mux(rx *plan9.Fcall) {
	c.x.Lock()
	defer c.x.Unlock()

	ch := c.tagmap[rx.Tag]
	delete(c.tagmap, rx.Tag)
	c.freetag[rx.Tag] = true
	c.muxer = false
	for _, ch2 := range c.tagmap {
		c.muxer = true
		ch2 <- &yourTurn
		break
	}
	ch <- rx
}

func (c *conn) read() (*plan9.Fcall, error) {
	if err := c.getErr(); err != nil {
		return nil, err
	}
	f, err := plan9.ReadFcall(c.rwc)
	if err != nil {
		c.setErr(err)
		return nil, err
	}
	return f, nil
}

func (c *conn) write(f *plan9.Fcall) error {
	if err := c.getErr(); err != nil {
		return err
	}
	err := plan9.WriteFcall(c.rwc, f)
	if err != nil {
		c.setErr(err)
	}
	return err
}

var yourTurn plan9.Fcall

func (c *conn) rpc(tx *plan9.Fcall, clunkFid *Fid) (rx *plan9.Fcall, err error) {
	ch := make(chan *plan9.Fcall, 1)
	tx.Tag, err = c.newtag(ch)
	if err != nil {
		return nil, err
	}
	c.w.Lock()
	err = c.write(tx)
	// Mark the fid as clunked inside the write lock so that we're
	// sure that we don't reuse it after the sending the message
	// that will clunk it, even in the presence of concurrent method
	// calls on Fid.
	if clunkFid != nil {
		// Closing the Fid might release the conn, which
		// would close the underlying rwc connection,
		// which would prevent us from being able to receive the
		// reply, so make sure that doesn't happen until the end
		// by acquiring a reference for the duration of the call.
		c.acquire()
		defer c.release()
		if err := clunkFid.clunked(); err != nil {
			// This can happen if two clunking operations
			// (e.g. Close and Remove) are invoked concurrently
			c.w.Unlock()
			return nil, err
		}
	}
	c.w.Unlock()
	if err != nil {
		return nil, err
	}

	for rx = range ch {
		if rx != &yourTurn {
			break
		}
		rx, err = c.read()
		if err != nil {
			break
		}
		c.mux(rx)
	}

	if rx == nil {
		return nil, c.getErr()
	}
	if rx.Type == plan9.Rerror {
		return nil, Error(rx.Ename)
	}
	if rx.Type != tx.Type+1 {
		return nil, plan9.ProtocolError("packet type mismatch")
	}
	return rx, nil
}

func (c *conn) acquire() {
	atomic.AddInt32(&c.refCount, 1)
}

func (c *conn) release() error {
	if atomic.AddInt32(&c.refCount, -1) != 0 {
		return nil
	}
	err := c.rwc.Close()
	c.setErr(errClosed)
	return err
}

func (c *conn) getErr() error {
	c.x.Lock()
	defer c.x.Unlock()
	return c.err
}

func (c *conn) setErr(err error) {
	c.x.Lock()
	defer c.x.Unlock()
	c.err = err
}
