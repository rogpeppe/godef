package client

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	plan9 "code.google.com/p/rog-go/new9p"
)

type Error string

func (e Error) Error() string { return string(e) }

type Conn struct {
	rwc     io.ReadWriteCloser
	err     error
	tagmap  map[uint16]chan *plan9.Fcall
	freetag map[uint16]bool
	freefid map[uint32]bool
	nexttag uint16
	nextfid uint32
	msize   uint32
	version string
	w, x    sync.Mutex
}

func NewConn(rwc io.ReadWriteCloser) (*Conn, error) {
	c := &Conn{
		rwc:     rwc,
		tagmap:  make(map[uint16]chan *plan9.Fcall),
		freetag: make(map[uint16]bool),
		freefid: make(map[uint32]bool),
		nexttag: 1,
		nextfid: 1,
		msize:   64 * 1024,
		version: "9P2000",
	}

	go c.muxer()

	//	XXX raw messages, not c.rpc
	tx := &plan9.Fcall{Type: plan9.Tversion, Msize: c.msize, Version: c.version}
	rx, err := c.rpc(tx)
	if err != nil {
		return nil, err
	}

	if rx.Msize > c.msize {
		return nil, plan9.ProtocolError(fmt.Sprintf("invalid msize %d in Rversion", rx.Msize))
	}
	if rx.Version != "9P2000" {
		return nil, plan9.ProtocolError(fmt.Sprintf("invalid version %s in Rversion", rx.Version))
	}
	c.msize = rx.Msize
	return c, nil
}

func (c *Conn) muxer() {
	for {
		rx, _ := c.read()
		if rx == nil {
			break
		}
		c.mux(rx)
	}
	c.x.Lock()
	for _, ch := range c.tagmap {
		ch <- nil
	}
	c.x.Unlock()
}

func (c *Conn) mux(rx *plan9.Fcall) {
	c.x.Lock()
	ch := c.tagmap[rx.Tag]
	if ch == nil {
		fmt.Fprintf(os.Stderr, "unknown tag in Rmsg: %v", rx)
		c.x.Unlock()
		return
	}
	c.x.Unlock()
	ch <- rx
}

func (c *Conn) getfid() (*Fid, error) {
	c.x.Lock()
	defer c.x.Unlock()
	var fidnum uint32
	//	for fidnum, _ = range c.freefid {
	//		c.freefid[fidnum] = false, false
	//		goto found
	//	}
	fidnum = c.nextfid
	if c.nextfid == plan9.NOFID {
		return nil, plan9.ProtocolError("out of fids")
	}
	c.nextfid++
	//found:
	fid := new(Fid)
	fid.fid = fidnum
	fid.c = c
	return fid, nil
}

func (c *Conn) putfid(f *Fid) {
	if c == nil {
		return
	}
	c.x.Lock()
	defer c.x.Unlock()
	if f.fid != 0 && f.fid != plan9.NOFID {
		c.freefid[f.fid] = true
		f.fid = plan9.NOFID
		f.flags = 0
		f.c = nil
	}
}

func (c *Conn) newtag(ch chan *plan9.Fcall) (uint16, error) {
	c.x.Lock()
	defer c.x.Unlock()
	var tagnum uint16
	//	for tagnum, _ = range c.freetag {
	//		c.freetag[tagnum] = false, false
	//		goto found
	//	}
	tagnum = c.nexttag
	if c.nexttag == plan9.NOTAG {
		return 0, plan9.ProtocolError("out of tags")
	}
	c.nexttag++
	//found:
	c.tagmap[tagnum] = ch
	return tagnum, nil
}

func (c *Conn) read() (*plan9.Fcall, error) {
	if err := c.getErr(); err != nil {
		return nil, err
	}
	f, err := plan9.ReadFcall(c.rwc)
	if err != nil {
		log.Printf("<-- read error: %v", err)
		c.setErr(err)
		return nil, err
	}
	//log.Printf("<-- %v", f)
	return f, nil
}

func (c *Conn) write(f *plan9.Fcall) error {
	if err := c.getErr(); err != nil {
		return err
	}
	//log.Printf("--> %v", f)
	err := plan9.WriteFcall(c.rwc, f)
	if err != nil {
		c.setErr(err)
	}
	return err
}

func (c *Conn) rpc(tx *plan9.Fcall) (rx *plan9.Fcall, err error) {
	ch := make(chan *plan9.Fcall, 1)
	tx.Tag, err = c.newtag(ch)
	if err != nil {
		return nil, err
	}

	c.w.Lock()
	if err := c.write(tx); err != nil {
		c.w.Unlock()
		return nil, err
	}
	c.w.Unlock()

	rx = <-ch
	if rx == nil {
		//log.Printf("rpc failed, closed %v, err %v\n", closed(ch), c.getErr())
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

func (c *Conn) Close() error {
	return c.rwc.Close()
}

func (c *Conn) getErr() error {
	c.x.Lock()
	err := c.err
	c.x.Unlock()
	return err
}

func (c *Conn) setErr(err error) {
	c.x.Lock()
	c.err = err
	c.x.Unlock()
}
