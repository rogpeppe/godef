// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The srv package provides definitions and functions used to implement
// a 9P2000 file client.
package g9pc

import (
	"code.google.com/p/rog-go/go9p/g9p"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"syscall"
)

// The Client type represents a 9P2000 client. The client is connected to
// a 9P2000 file server and its methods can be used to access and manipulate
// the files exported by the server.
type Client struct {
	mu    sync.Mutex
	msize uint32 // Maximum size of the 9P messages
	dotu  bool   // If true, 9P2000.u protocol is spoken
	root  *Fid   // Fid that points to the rood directory
	log   g9p.Logger

	finished chan error // client is no longer connected to server (holds error if any)
	conn     io.ReadWriteCloser
	tagpool  *pool
	fidpool  *pool
	reqout   chan *req
	reqfirst *req
	reqlast  *req
	//	err      *g9p.Error

	reqchan chan *req
	tchan   chan *g9p.Fcall

	next, prev *Client
}

// A Fid type represents a file on the server. Fids are used for the
// low level methods that correspond directly to the 9P2000 message requests
type Fid struct {
	mu       sync.Mutex
	Client   *Client // Client the fid belongs to
	Iounit   uint32
	g9p.Qid         // The Qid description for the file
	Mode     uint8  // Open mode (one of g9p.O* values) (if file is open)
	Fid      uint32 // Fid number
	g9p.User        // The user the fid belongs to
	walked   bool   // true if the fid points to a walked file on the server
}

// The file is similar to the Fid, but is used in the high-level client
// interface.
type File struct {
	fid    *Fid
	offset uint64
}

type pool struct {
	mu    sync.Mutex
	need  int
	nchan chan uint32
	maxid uint32
	imap  []byte
}

type req struct {
	mu         sync.Mutex
	client     *Client
	tc         *g9p.Fcall
	rc         *g9p.Fcall
	err        error
	done       chan *req
	tag        uint16
	prev, next *req
}

func (client *Client) rpcnb(r *req) error {
	var tag uint16
	if r.tc.Type == g9p.Tversion {
		tag = g9p.NOTAG
	} else {
		tag = r.tag
	}

	g9p.SetTag(r.tc, tag)
	client.mu.Lock()

	if client.reqlast != nil {
		client.reqlast.next = r
	} else {
		client.reqfirst = r
	}

	r.prev = client.reqlast
	client.reqlast = r
	client.mu.Unlock()

	select {
	case e := <-client.finished:
		client.finished <- e
		if e == nil {
			e = errors.New("Client no longer connected")
		}
		return e
	case client.reqout <- r:
	}

	return nil
}

func (client *Client) Msize() uint32 {
	return client.msize
}

func (client *Client) rpc(tc *g9p.Fcall) (rc *g9p.Fcall, err error) {
	r := client.reqAlloc()
	r.tc = tc
	r.done = make(chan *req)
	err = client.rpcnb(r)
	if err != nil {
		return
	}
	select {
	case <-r.done:
	case e := <-client.finished:
		client.finished <- e
		r.rc = nil
		r.err = e
	}
	rc = r.rc
	err = r.err
	client.reqFree(r)
	return
}

func (client *Client) recv() {
	var err error
	buf := make([]byte, client.msize*8)
	pos := 0
	for {
		if len(buf) < int(client.msize) {
		resize:
			b := make([]byte, client.msize*8)
			copy(b, buf[0:pos])
			buf = b
			b = nil
		}

		n, oerr := client.conn.Read(buf[pos:len(buf)])
		if oerr != nil || n == 0 {
			err = &g9p.Error{oerr.String(), syscall.EIO}
			goto closed
		}

		pos += n
		for pos > 4 {
			sz, _ := g9p.Gint32(buf)
			if pos < int(sz) {
				if len(buf) < int(sz) {
					goto resize
				}

				break
			}

			fc, oerr, fcsize := g9p.Unpack(buf, client.dotu)
			client.mu.Lock()
			if oerr != nil {
				err = oerr
				client.conn.Close()
				client.mu.Unlock()
				goto closed
			}

			if client.log != nil {
				f := new(g9p.Fcall)
				*f = *fc
				f.Pkt = nil
				client.log.Log9p(f)
			}

			var r *req = nil
			for r = client.reqfirst; r != nil; r = r.next {
				if r.tc.Tag == fc.Tag {
					break
				}
			}

			if r == nil {
				err = errors.New("unexpected response")
				client.conn.Close()
				client.mu.Unlock()
				goto closed
			}

			r.rc = fc
			if r.prev != nil {
				r.prev.next = r.next
			} else {
				client.reqfirst = r.next
			}

			if r.next != nil {
				r.next.prev = r.prev
			} else {
				client.reqlast = r.prev
			}
			client.mu.Unlock()

			if r.tc.Type != r.rc.Type-1 {
				if r.rc.Type != g9p.Rerror {
					r.err = &g9p.Error{"invalid response", syscall.EINVAL}
					log.Printf(fmt.Sprintf("TTT %v", r.tc))
					log.Printf(fmt.Sprintf("RRR %v", r.rc))
				} else {
					if r.err != nil {
						r.err = &g9p.Error{r.rc.Error, int(r.rc.Errornum)}
					}
				}
			}

			r.done <- r

			pos -= fcsize
			buf = buf[fcsize:]
		}
	}

closed:
	log.Printf("recv done\n")
	client.finished <- err

	/* send error to all pending requests */
	client.mu.Lock()
	client.reqfirst = nil
	client.reqlast = nil
	client.mu.Unlock()
}

func (client *Client) Wait() (err error) {
	err = <-client.finished
	client.finished <- err
	return
}

func (client *Client) send() {
	for {
		select {
		case err := <-client.finished:
			client.finished <- err
			return

		case r := <-client.reqout:
			if client.log != nil {
				client.log.Log9p(r.tc)
			}
			for buf := r.tc.Pkt; len(buf) > 0; {
				n, err := client.conn.Write(buf)
				if err != nil {
					/* just close the socket, will get signal on client.finished */
					client.conn.Close()
					break
				}

				buf = buf[n:]
			}
		}
	}
}

// NewClient creates a client object for the 9p server connected
// to by c. It negotiates the dialect and msize for the
// connection. Returns a Client object, or Error.
func NewClient(c io.ReadWriteCloser, msize uint32, dotu bool, log g9p.Logger) (*Client, error) {
	client := new(Client)
	client.conn = c
	client.msize = msize
	client.dotu = dotu
	client.tagpool = newPool(uint32(g9p.NOTAG))
	client.fidpool = newPool(g9p.NOFID)
	client.reqout = make(chan *req)
	client.finished = make(chan error, 1)
	client.reqchan = make(chan *req, 16)
	client.tchan = make(chan *g9p.Fcall, 16)
	client.log = log
	ver := "9P2000"
	if client.dotu {
		ver = "9P2000.u"
	}

	go client.recv()
	go client.send()

	tc := g9p.NewFcall(client.msize)
	err := g9p.PackTversion(tc, client.msize, ver)
	if err != nil {
		goto error
	}

	rc, err := client.rpc(tc)
	if err != nil {
		goto error
	}

	if rc.Msize < client.msize {
		client.msize = rc.Msize
	}

	client.dotu = rc.Version == "9P2000.u" && client.dotu
	return client, nil

error:
	if log != nil {
		log.Log9p(nil)
	}
	return nil, err
}

// Creates a new Fid object for the client
func (client *Client) fidAlloc() *Fid {
	fid := new(Fid)
	fid.Fid = client.fidpool.getId()
	fid.Client = client

	return fid
}

func (client *Client) newFcall() *g9p.Fcall {
	select {
	case tc := <-client.tchan:
		return tc
	default:
	}
	return g9p.NewFcall(client.msize)
}

func (client *Client) reqAlloc() *req {
	select {
	case r := <-client.reqchan:
		return r
	default:
	}
	r := new(req)
	r.client = client
	r.tag = uint16(client.tagpool.getId())

	return r
}

func (client *Client) reqFree(r *req) {
	if r.tc != nil && len(r.tc.Buf) >= int(client.msize) {
		select {
		case client.tchan <- r.tc:
		default:
		}
	}

	r.tc = nil
	r.rc = nil
	r.err = nil
	r.done = nil
	r.next = nil
	r.prev = nil

	select {
	case client.reqchan <- r:
	default:
		client.tagpool.putId(uint32(r.tag))
	}
}
