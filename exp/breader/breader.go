package breader

import (
	"io"
	"io/ioutil"
	"os"
)

type bufferedReader struct {
	req   chan []byte
	reply chan int
	tmpf  *os.File
	error error
}

const ioUnit = 16 * 1024

// NewReader continually reads from in and buffers the data
// inside a temporary file (created with ioutil.TempFile(dir, prefix)).
// It returns a Reader that can be used to read the data.
func NewReader(in io.Reader, dir, prefix string) (io.Reader, error) {
	br := new(bufferedReader)
	br.error = io.EOF
	tmpf, err := ioutil.TempFile(dir, prefix)
	if tmpf == nil {
		return nil, err
	}
	br.tmpf = tmpf
	br.req = make(chan []byte, 1)
	br.reply = make(chan int)

	nread := make(chan int)
	go br.reader(in, nread)
	go br.buffer(nread)
	return br, nil
}

func (br *bufferedReader) Read(buf []byte) (int, error) {
	br.req <- buf
	n := <-br.reply
	if n == 0 {
		return 0, br.error
	}
	return n, nil
}

func (br *bufferedReader) reader(r io.Reader, nread chan<- int) {
	buf := make([]byte, ioUnit)
	off := int64(0)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			n, err = br.tmpf.WriteAt(buf[0:n], off)
			nread <- n
			off += int64(n)
		}
		// TODO if r.Read returns >0, err!=nil, then we'll miss the error and read again,
		if err != nil {
			br.error = err
			close(nread)
			return
		}
	}
}

func (br *bufferedReader) buffer(nread <-chan int) {
	roff := int64(0)
	woff := int64(0)
	defer close(br.reply)
	for {
		req := br.req
		space := woff - roff
		if space <= 0 {
			// if there's nothing left in the buffer and we've received
			// EOF, then we're done.
			if nread == nil {
				return
			}
			req = nil
		}
		select {
		case buf := <-req:
			if int64(len(buf)) > space {
				buf = buf[0:space]
			}
			n, err := br.tmpf.ReadAt(buf, roff)
			if err != nil {
				br.error = err
				return
			}
			br.reply <- n
			roff += int64(n)
		case n := <-nread:
			if closed(nread) {
				nread = nil
			}
			woff += int64(n)
		}
	}
}
