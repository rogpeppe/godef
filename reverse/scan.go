package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"io"
	"strings"
)

var txt = `
hello one two three four five six seven
there

you one two three four five six seven` + "\r" + `
x

`

func main() {
	b := NewReverseScanner(strings.NewReader(txt))
	b.Split(bufio.ScanWords)
	for b.Scan() {
		fmt.Printf("got %q\n", b.Bytes())
	}
	if b.Err() != nil {
		fmt.Printf("final error %v\n", b.Err())
	}
}

const maxBufSize = 64 * 1024

type RevScanner struct {
	r io.ReadSeeker

	split bufio.SplitFunc

	// offset holds the file offset of the data
	// in buf.
	offset int64

	// atEOF reports whether the buffer
	// is located at the end of the file.
	atEOF bool

	// buf holds currently buffered data.
	buf []byte

	// tokens holds any currently unprocessed
	// tokens in buf.
	tokens [][]byte

	// partialToken holds the size of the partial
	// token at the start of buf.
	partialToken int

	// err holds any error encountered.
	err error
}

func NewReverseScanner(r io.ReadSeeker) *RevScanner {
	b := &RevScanner{
		r:     r,
		buf:   make([]byte, 20),
		atEOF: true,
		split: bufio.ScanLines,
	}
	b.offset, b.err = r.Seek(0, 2)
	log.Printf("initial offset %d", b.offset)
	return b
}

func (b *RevScanner) fillbuf() error {
	log.Printf("fillbuf offset %d; partial %d;", b.offset, b.partialToken)
	b.tokens = b.tokens[:0]
	if b.offset == 0 {
		return io.EOF
	}
	// Copy any partial token data to the end of the buffer.
	space := len(b.buf) - b.partialToken
	if space == 0 {
		log.Printf("no space")
		if len(b.buf) >= maxBufSize {
			return errors.New("token too long")
		}
		n := len(b.buf) * 2
		if n > maxBufSize {
			n = maxBufSize
		}
		log.Printf("expanding buffer to %d", n)
		// Partial token fills the buffer, so expand it.
		newBuf := make([]byte, n)
		copy(newBuf, b.buf[0:b.partialToken])
		b.buf = newBuf
		space = len(b.buf) - b.partialToken
	}
	if int64(space) > b.offset {
		// We have less than the given buffer's space
		// left to read, so shrink the buffer to simplify
		// the remaining logic by preserving the
		// invariants that b.tokens[0] + space == len(buf)
		// and the data is read into the start of buf.
		b.buf = b.buf[0 : b.partialToken+int(b.offset)]
		space = len(b.buf) - b.partialToken
		log.Printf("shrunk buf to %d; space %d", len(b.buf), space)
	}
	newOffset := b.offset - int64(space)
	if newOffset < 0 {
		panic("negative offset")
	}
	log.Printf("copying %d bytes to %d (%q)", b.partialToken, space, b.buf[0:b.partialToken])
	// Copy old partial token to end of buffer.
	copy(b.buf[space:], b.buf[0:b.partialToken])
	log.Printf("reading %d bytes at %d", len(b.buf) - b.partialToken, newOffset)
	_, err := b.r.Seek(newOffset, 0)
	if err != nil {
		return err
	}
	b.offset = newOffset
	if _, err := io.ReadFull(b.r, b.buf[0:space]); err != nil {
		return err
	}
	log.Printf("read %q", b.buf[0:space])
	log.Printf("scanning buf %q", b.buf)
	// Populate tokens.
	if b.offset > 0 {
		// We're not at the start of the file - read the first
		// token to find out where the token boundary is, but we
		// don't treat it as an actual token, because we're
		// probably not at its start.
		advance, token, err := b.split(b.buf, b.atEOF)
		log.Printf("scan initial -> %d, %q, %v", advance, token, err)
		if err != nil {
			// If the split function can return an error
			// when starting at a non-token boundary, this
			// will happen and there's not much we can do
			// about it other than scanning forward a byte
			// at a time in a horribly inefficient manner.
			return err
		}
		if advance == 0 {
			return errors.New("unexpected incomplete token")
		}
		b.partialToken = advance
		if b.partialToken == len(b.buf) {
			// There are no more tokens in the buffer,
			// so try again (the buffer will expand)
			return b.fillbuf()
		}
	} else {
		b.partialToken = 0
	}
	i := b.partialToken
	for i < len(b.buf) {
		advance, token, err := b.split(b.buf[i:], b.atEOF)
		log.Printf("scan %q at %d (eof %v)-> %d, %q, %v", b.buf[i:], i, b.atEOF, advance, token, err)
		if err != nil {
			return err
		}
		if advance == 0 {
			// There's no remaining token in the buffer.
			break
		}
		b.tokens = append(b.tokens, token)
		i += advance
	}
	b.atEOF = false
	return nil
}

func (b *RevScanner) Scan() bool {
	if len(b.tokens) > 0 {
		b.tokens = b.tokens[0 : len(b.tokens)-1]
	}
	if len(b.tokens) > 0 {
		return true
	}
	if b.err != nil {
		return false
	}
	b.err = b.fillbuf()
	return len(b.tokens) > 0
}

func (b *RevScanner) Split(split bufio.SplitFunc) {
	b.split = split
}

func (b *RevScanner) Bytes() []byte {
	return b.tokens[len(b.tokens)-1]
}

func (b *RevScanner) Text() string {
	return string(b.Bytes())
}

func (b *RevScanner) Err() error {
	if len(b.tokens) > 0 {
		return nil
	}
	if b.err == io.EOF {
		return nil
	}
	return b.err
}
