// The netchanrpc package makes it possible to run an RPC service
// over netchan.
package ncnet

import (
	"fmt"
	"log"
	"net"
	"netchan"
	"os"
)

const initMessage = "netconnect"

type netchanAddr string

type hanguper interface {
	Hangup(name string) os.Error
}

func (a netchanAddr) String() string {
	return string(a)
}

func (a netchanAddr) Network() string {
	return "netchan"
}

type netchanConn struct {
	*chanReader
	*chanWriter
	clientName string
	localAddr netchanAddr
	remoteAddr netchanAddr
	nc hanguper
}

func (c *netchanConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *netchanConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *netchanConn) SetReadTimeout(nsec int64) os.Error {
	return os.ErrorString("cannot set timeout")
}

func (c *netchanConn) SetWriteTimeout(nsec int64) os.Error {
	return os.ErrorString("cannot set timeout")
}

func (c *netchanConn) SetTimeout(nsec int64) os.Error {
	return os.ErrorString("cannot set timeout")
}

func (c *netchanConn) Close() os.Error {
	c.nc.Hangup(c.clientName + ".req")
	c.nc.Hangup(c.clientName + ".reply")
	return nil
}

type netchanListener struct {
	exp *netchan.Exporter
	name string
	conns chan net.Conn
	err os.Error
	closed chan bool			// closed when closed; never sent on otherwise.
}

// Listen uses the given Exporter to listen on the given service name.
// It uses a set of netchan channels, all prefixed with that name.
func Listen(exp *netchan.Exporter, service string) (net.Listener, os.Error) {
	r := &netchanListener{
		exp: exp,
		name: service,
		conns: make(chan net.Conn),
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
			select{
			case clientNames <- clientName:
			case <-r.closed:
				return
			}
		}
	}()
	return r, nil
}

func (r *netchanListener) Accept() (c net.Conn, err os.Error) {
	c = <-r.conns
	if closed(r.conns) {
		err = r.err
	}
	return
}

func (r *netchanListener) Close() os.Error {
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
		c := &netchanConn{
			chanReader: newChanReader(req),
			chanWriter: newChanWriter(reply),
			clientName: clientName,
			localAddr: netchanAddr(r.name),
			remoteAddr: netchanAddr("unknown"),
			nc: r.exp,
		}
		select{
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
		select{
		case r.conns <- c:
		case <-r.closed:
			c.Close()
		}
	}()
}

// Dial makes a connection to the named netchan service,
// which must have been previously exported with a call to Listen.
func Dial(imp *netchan.Importer, service string) (net.Conn, os.Error) {
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
	return &netchanConn{
		chanReader: &chanReader{c: reply},
		chanWriter: &chanWriter{c: req},
		clientName: clientName,
		localAddr: netchanAddr("unknown"),
		remoteAddr: netchanAddr(service),
		nc: imp,
	}, nil
}

type chanReader struct {
	buf []byte
	c   <-chan []byte
}

func newChanReader(c <-chan []byte) *chanReader {
	return &chanReader{c: c}
}

func (r *chanReader) Read(buf []byte) (int, os.Error) {
	for len(r.buf) == 0 {
		r.buf = <-r.c
		if closed(r.c) {
			return 0, os.EOF
		}
	}
	n := copy(buf, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

type chanWriter struct {
	c chan<- []byte
}

func newChanWriter(c chan<- []byte) *chanWriter {
	return &chanWriter{c: c}
}
func (w *chanWriter) Write(buf []byte) (n int, err os.Error) {
	b := make([]byte, len(buf))
	copy(b, buf)
	w.c <- b
	return len(buf), nil
}
