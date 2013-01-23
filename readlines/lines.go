package readlines

import (
	"bufio"
	"io"
	"unicode/utf8"
)

// Iter reads lines from r and calls fn with each read line, not
// including the line terminator.  If a line exceeds the given maximum
// size, it will be truncated and the rest of the line discarded.  If fn
// returns a non-nil error, reading ends and the error is returned from
// Iter.  When EOF is encountered, Read returns nil.
func Iter(r io.Reader, maxSize int, fn func(line []byte) error) error {
	b := bufio.NewReader(r)
	for {
		line, isPrefix, err := b.ReadLine()
		if err != nil {
			return eofNilError(err)
		}
		if !isPrefix {
			// Simple line that fits within the bufio buffer size.
			if err := fn(truncate(line, maxSize)); err != nil {
				return err
			}
			continue
		}
		buf := make([]byte, len(line), len(line)*2)
		copy(buf, line)
		for isPrefix {
			line, isPrefix, err = b.ReadLine()
			if err != nil {
				if err := fn(buf); err != nil {
					return err
				}
				return eofNilError(err)
			}
			buf = append(buf, line...)
			if len(buf) >= maxSize {
				break
			}
		}
		if err := fn(truncate(line, maxSize)); err != nil {
			return err
		}
		// Discard any of the line that exceeds the maximum size
		for isPrefix {
			_, isPrefix, err = b.ReadLine()
			if err != nil {
				return eofNilError(err)
			}
		}
	}
	panic("unreachable")
}

// truncate returns s truncated to the given size,
// avoiding splitting a multibyte UTF-8 sequence.
func truncate(p []byte, size int) []byte {
	if len(p) <= size {
		return p
	}
	p = p[0:size]
	start := size - 1
	r := rune(p[start])
	if r < utf8.RuneSelf {
		return p
	}
	// Find the start of the last character and check
	// whether it's valid.
	lim := size - utf8.UTFMax
	if lim < 0 {
		lim = 0
	}
	for ; start >= lim; start-- {
		if utf8.RuneStart(p[start]) {
			break
		}
	}
	// If we can't find the start of the last character,
	// return the whole lot.
	if start < 0 {
		return p
	}
	r, rsize := utf8.DecodeRune(p[start:size])
	// The last rune was valid, so include it.
	if rsize > 1 {
		return p
	}
	// The last rune was invalid, so lose it.
	return p[0:start]
}

func eofNilError(err error) error {
	if err == io.EOF {
		return nil
	}
	return err
}
