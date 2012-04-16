package basic

import (
	"code.google.com/p/rog-go/exp/abc"
	"io"
)

func Use() {
}

// male side sends reader (or nil if it has no preference);
// female replies with the actual fd to use (or nil if there's been an error)
var FdT = &abc.Type{"fd", false, func(x interface{}) (ok bool) { _, ok = x.(Fd); return }}

type Fd chan fdRequest

type fdRequest struct {
	r  io.Reader
	wc chan io.Writer
}

type nullWidget struct{}

func (_ nullWidget) Plug(_ string, _ interface{}) {
}

// male side
func (fd Fd) GetWriter(r io.Reader) io.Writer {
	reply := make(chan io.Writer)
	fd <- fdRequest{r, reply}
	return <-reply
}

// female side
func (fd Fd) GetReader() io.Reader {
	req := <-fd
	if req.r == nil {
		r, w := io.Pipe()
		req.wc <- w
		req.r = r
	} else {
		req.wc <- nil // reader accepted; other end can now disappear
	}
	return req.r
}

// female side
func (fd Fd) PutWriter(w io.Writer) {
	req := <-fd
	req.wc <- w
}

func NewFd() Fd {
	return make(chan fdRequest)
}

func close(x interface{}) {
	if x, ok := x.(io.Closer); ok {
		x.Close()
	}
}
