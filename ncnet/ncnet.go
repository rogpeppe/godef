// The netchanrpc package makes it possible to run an RPC service
// over netchan.
package ncnet

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"netchan"
)

const initMessage = "netconnect"

type netchanAddr string

type hanguper interface {
	Hangup(name string) error
}

func (a netchanAddr) String() string {
	return string(a)
}

func (a netchanAddr) Network() string {
	return "netchan"
}

// Conn represents a netchan connection.
// R and W hold the channels used by the connection.
// The Read and Write methods use them to receive
// and send data. The W channel should not be
// closed - it will be closed when Close is called
// on the connection itself.
type Conn struct {
	R <-chan []byte
	W chan<- []byte
	*chanReader
	*chanWriter
	clientName string
	localAddr  netchanAddr
	remoteAddr netchanAddr
	nc         hanguper
}

func (c *Conn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *Conn) SetReadTimeout(nsec int64) error {
	return errors.New("cannot set timeout")
}

func (c *Conn) SetWriteTimeout(nsec int64) error {
	return errors.New("cannot set timeout")
}

func (c *Conn) SetTimeout(nsec int64) error {
	return errors.New("cannot set timeout")
}

func (c *Conn) Close() error {
	c.nc.Hangup(c.clientName + ".req")
	c.nc.Hangup(c.clientName + ".reply")
	return nil
}

type netchanListener struct {
	exp    *netchan.Exporter
	name   string
	conns  chan net.Conn
	err    error
	closed chan bool // closed when closed; never sent on otherwise.
}

// Listen uses the given Exporter to listen on the given service name.
// It uses a set of netchan channels, all prefixed with that name.
// The connections returned by the Listener have underlying type *Conn.
// This can be used to gain access to the underlying channels.
func Listen(exp *netchan.Exporter, service string) (net.Listener, error) {
	r := &netchanListener{
		exp:    exp,
		name:   service,
		conns:  make(chan net.Conn),
		closed: make(chan bool),
	}
	// Create the auxilliary channel and export it.
	clientNames := make(chan string)
	err := exp.Export(service, clientNames, netchan.Send)
	if err != nil {
		return nil, err
	}
	go func() {
		for i := 0; ; i++ {
			clientName := fmt.Sprintf("%s.%d", service, i)
			r.exporter(clientName)
			select {
			case clientNames <- clientName:
			case <-r.closed:
				return
			}
		}
	}()
	return r, nil
}

func (r *netchanListener) Accept() (c net.Conn, err error) {
	c, ok := <-r.conns
	if !ok {
		err = r.err
	}
	return
}

func (r *netchanListener) Close() error {
	close(r.closed)
	return nil
}

func (r *netchanListener) Addr() net.Addr {
	return netchanAddr(r.name)
}

// One exporter runs for each client.
func (r *netchanListener) exporter(clientName string) {
	req, reqname := make(chan []byte), clientName+".req"
	reply, replyname := make(chan []byte), clientName+".reply"
	err := r.exp.Export(reqname, req, netchan.Recv)
	if err != nil {
		log.Printf("cannot export %q: %v", reqname, err)
		return
	}
	err = r.exp.Export(replyname, reply, netchan.Send)
	if err != nil {
		log.Printf("cannot export %q: %v", replyname, err)
		r.exp.Hangup(reqname)
		return
	}

	go func() {
		c := &Conn{
			R:          req,
			W:          reply,
			chanReader: newChanReader(req),
			chanWriter: newChanWriter(reply),
			clientName: clientName,
			localAddr:  netchanAddr(r.name),
			remoteAddr: netchanAddr("unknown"),
			nc:         r.exp,
		}
		select {
		case m := <-req:
			if string(m) != initMessage {
				r.exp.Hangup(reqname)
				r.exp.Hangup(replyname)
				return
			}
		case <-r.closed:
			c.Close()
			return
		}
		// BUG: there's no way for us to tell when a client goes away
		// unless they close the channel, so we will leak exporters
		// where the importer is killed.
		select {
		case r.conns <- c:
		case <-r.closed:
			c.Close()
		}
	}()
}

// Dial makes a connection to the named netchan service,
// which must have been previously exported with a call to Listen.
func Dial(imp *netchan.Importer, service string) (net.Conn, error) {
	cnames := make(chan string)
	err := imp.ImportNValues(service, cnames, netchan.Recv, 1, 1)
	if err != nil {
		return nil, err
	}
	clientName := <-cnames
	reqname := clientName + ".req"
	replyname := clientName + ".reply"
	req := make(chan []byte)
	err = imp.Import(reqname, req, netchan.Send, 200)
	if err != nil {
		return nil, err
	}
	reply := make(chan []byte)
	err = imp.Import(replyname, reply, netchan.Recv, 200)
	if err != nil {
		return nil, err
	}
	req <- []byte(initMessage)
	return &Conn{
		R:          reply,
		W:          req,
		chanReader: &chanReader{c: reply},
		chanWriter: &chanWriter{c: req},
		clientName: clientName,
		localAddr:  netchanAddr("unknown"),
		remoteAddr: netchanAddr(service),
		nc:         imp,
	}, nil
}

// chanReader receives on the channel when its
// Read method is called. Extra data received is
// buffered until read.
type chanReader struct {
	buf []byte
	c   <-chan []byte
}

func newChanReader(c <-chan []byte) *chanReader {
	return &chanReader{c: c}
}

func (r *chanReader) Read(buf []byte) (int, error) {
	for len(r.buf) == 0 {
		var ok bool
		r.buf, ok = <-r.c
		if !ok {
			return 0, io.EOF
		}
	}
	n := copy(buf, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

// chanWriter writes on the channel when its
// Write method is called.
type chanWriter struct {
	c chan<- []byte
}

func newChanWriter(c chan<- []byte) *chanWriter {
	return &chanWriter{c: c}
}
func (w *chanWriter) Write(buf []byte) (n int, err error) {
	b := make([]byte, len(buf))
	copy(b, buf)
	w.c <- b
	return len(buf), nil
}
