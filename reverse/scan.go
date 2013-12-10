package reverse

import (
	"bufio"
	"errors"
	"io"
)

const maxBufSize = 64 * 1024

type Scanner struct {
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

func NewScanner(r io.ReadSeeker) *Scanner {
	b := &Scanner{
		r:     r,
		buf:   make([]byte, 20),
		atEOF: true,
		split: bufio.ScanLines,
	}
	b.offset, b.err = r.Seek(0, 2)
	return b
}

func (b *Scanner) fillbuf() error {
	b.tokens = b.tokens[:0]
	if b.offset == 0 {
		return io.EOF
	}
	// Copy any partial token data to the end of the buffer.
	space := len(b.buf) - b.partialToken
	if space == 0 {
		if len(b.buf) >= maxBufSize {
			return errors.New("token too long")
		}
		n := len(b.buf) * 2
		if n > maxBufSize {
			n = maxBufSize
		}
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
	}
	newOffset := b.offset - int64(space)
	if newOffset < 0 {
		panic("negative offset")
	}
	// Copy old partial token to end of buffer.
	copy(b.buf[space:], b.buf[0:b.partialToken])
	_, err := b.r.Seek(newOffset, 0)
	if err != nil {
		return err
	}
	b.offset = newOffset
	if _, err := io.ReadFull(b.r, b.buf[0:space]); err != nil {
		return err
	}
	// Populate tokens.
	if b.offset > 0 {
		// We're not at the start of the file - read the first
		// token to find out where the token boundary is, but we
		// don't treat it as an actual token, because we're
		// probably not at its start.
		advance, _, err := b.split(b.buf, b.atEOF)
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

func (b *Scanner) Scan() bool {
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

func (b *Scanner) Split(split bufio.SplitFunc) {
	b.split = split
}

func (b *Scanner) Bytes() []byte {
	return b.tokens[len(b.tokens)-1]
}

func (b *Scanner) Text() string {
	return string(b.Bytes())
}

func (b *Scanner) Err() error {
	if len(b.tokens) > 0 {
		return nil
	}
	if b.err == io.EOF {
		return nil
	}
	return b.err
}
