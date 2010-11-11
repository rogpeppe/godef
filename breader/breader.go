package breader

import (
	"io"
	"os"
	"io/ioutil"
)

type BufferedReader struct {
	in io.Reader
	free *block
	maxOffset int64
	tmpf *os.File
	reqch chan []byte
	reply chan int
	error os.Error
}

type block struct {
	offset int64
	len int
	next *block
}

const ioUnit = 16*1024
const blockSize = 1024*1024

// NewReader continually reads from in and buffers the data
// inside a temporary file (created with ioutil.TempFile(dir, prefix)).
// It returns a ReadCloser that can be used to read the data.
func NewReader(in io.Reader, dir, prefix string) (io.ReadCloser, os.Error) {
	br := &BufferedReader{in: in}
	tmpf, err := ioutil.TempFile(dir, prefix)
	if tmpf == nil {
		return nil, err
	}
	br.tmpf = tmpf
	datach := make(chan []byte)
	br.reqch = make(chan []byte, 1)
	br.reply = make(chan int)
	go br.reader(in, datach)
	go br.buffer(br.reqch, datach)
	return br, nil
}

func (b *BufferedReader) reader(r io.Reader, datach chan<- []byte) {
	for {
		buf := make([]byte, ioUnit)
		n, err := r.Read(buf)
		if n > 0 {
			datach <- buf[0:n]
		}
		if err != nil {
			b.error = err
			close(datach)
			return
		}
	}
}

func (br *BufferedReader) Read(buf []byte) (int, os.Error) {
	if br.reqch == nil {
		return 0, os.EOF
	}
	br.reqch <- buf
	n := <-br.reply
	if closed(br.reply) {
		return 0, br.error
	}
	return n, nil
}

func (br *BufferedReader) Close() os.Error {
	if br.reqch == nil {
		return nil
	}
	close(br.reqch)
	br.reqch = nil
	return nil
}

func (br *BufferedReader) buffer(reqch, datach <-chan []byte) {
	defer close(br.reply)
	last := br.getBlock()	// last block in queue, accumulating data.
	first := last		// first block in queue, always non-nil.
	nread := 0			// number of bytes read from first.
	for {
		reqch := reqch
		// Don't wait for read requests if there's nothing to send.
		if datach != nil && first.next == nil && nread == first.len {
			reqch = nil
		}
		select {
		case data := <-datach:
			if closed(datach) {
				datach = nil
				break
			}
			for len(data) > 0 {
				n, err := br.writeBlock(last, data)
				if err != nil {
					br.error = err
					datach = nil
					break
				}
				if last.len >= blockSize {
					last.next = br.getBlock()
					last = last.next
				}
				data = data[n:]
			}

		case buf := <-reqch:
			if closed(reqch) {
				br.error = os.ErrorString("closed")
				return
			}
			n, err := br.readBlock(first, buf, nread)
			if err != nil {
				return
			}
			nread += n
			br.reply <- n

			// If we've read the entire contents of a block,
			// free it and move onto the next block.
			// It is guaranteed that there is always a next
			// block because we always allocate a new
			// one when last fills up.
			if nread == blockSize {
				next := first.next
				br.freeBlock(first)
				first = next
				nread = 0
			}
			if datach == nil && nread == first.len {
				return
			}
		}
	}
}

func (br *BufferedReader) getBlock() *block {
	if br.free == nil {
		b := &block{br.maxOffset, 0, nil}
		br.maxOffset += blockSize
		return b
	}
	b := br.free
	br.free = br.free.next
	b.len = 0
	b.next = nil
	return b
}

func (br *BufferedReader) freeBlock(b *block) {
	b.next = br.free
	br.free = b
}

// Write some data to the block b. The number of bytes written
// will be less than len(d) when the block fills up.
func (br *BufferedReader) writeBlock(b *block, data []byte) (int, os.Error) {
	space := blockSize - b.len
	if len(data) > space {
		data = data[0:space]
	}
	_, err := br.tmpf.WriteAt(data, b.offset+int64(b.len))
	if err != nil {
		return 0, err
	}
	b.len += len(data)
	return len(data), nil
}

// read from block b, starting at offset off within the block.
func  (br *BufferedReader) readBlock(b *block, data []byte, off int) (int, os.Error) {
	space := b.len - off;
	if len(data) > space {
		data = data[0:space]
	}
	return br.tmpf.ReadAt(data, b.offset+ int64(off))
}
