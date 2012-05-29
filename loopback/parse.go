package loopback

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
)

var errEmpty = errors.New("empty")

func parseNetwork(net string) (inOpts, outOpts Options, actualNet string, err error) {
	if net == "" {
		err = errors.New("empty network name")
		return
	}
	if net[0] != '[' {
		actualNet = net
		return
	}
	buf := bytes.NewBuffer([]byte(net[1:]))

	for {
		_, err = fmt.Fscan(buf, opts{&inOpts, &outOpts})
		if err != nil {
			if err == errEmpty {
				err = nil
				return
			}
			break
		}

		var r rune
		r, err = nextRune(buf)
		if err != nil {
			return
		}
		if r == ']' {
			break
		}
		if r != ',' {
			err = fmt.Errorf("badly formed options; expected ',' got '%c'", r)
			return
		}
	}
	actualNet = string(buf.Bytes())
	return
}

var _ = fmt.Scanner(opts{})

type opts struct {
	inOpts  *Options
	outOpts *Options
}

func (o opts) Scan(state fmt.ScanState, verb rune) error {
	var attr string
	var u unit
	if _, err := fmt.Fscanf(state, "%v=%v", (*word)(&attr), (*unit)(&u)); err != nil {
		return err
	}
	var in *Options
	var out *Options
	switch {
	case strings.HasPrefix(attr, "in."):
		in = o.inOpts
		attr = attr[3:]
	case strings.HasPrefix(attr, "out."):
		out = o.outOpts
		attr = attr[4:]
	default:
		in = o.inOpts
		out = o.outOpts
	}
	if err := setOpt(in, attr, u); err != nil {
		return err
	}
	if err := setOpt(out, attr, u); err != nil {
		return err
	}
	return nil
}

func setOpt(opt *Options, attr string, t unit) error {
	if opt == nil {
		return nil
	}
	switch attr {
	case "latency":
		opt.Latency = time.Duration(t)
	case "bytedelay":
		opt.ByteDelay = time.Duration(t)
	case "mtu":
		opt.MTU = int(t)
	case "inlimit":
		opt.InLimit = int(t)
	case "outlimit":
		opt.OutLimit = int(t)
	default:
		return fmt.Errorf("unknown attribute %q", attr)
	}
	return nil
}

var _ = fmt.Scanner((*word)(nil))

type word string

func (w *word) Scan(state fmt.ScanState, verb rune) error {
	tok, err := state.Token(false, func(r rune) bool { return unicode.IsLetter(r) || r == '.' })
	if err == nil && len(tok) == 0 {
		return errEmpty
	}
	*w = word(tok)
	return err
}

var _ = fmt.Scanner((*word)(nil))

type unit int64

func (u *unit) Scan(state fmt.ScanState, verb rune) error {
	var x float64
	_, err := fmt.Fscan(state, &x)
	if err != nil {
		return err
	}
	tok, err := state.Token(false, unicode.IsLetter)
	if err != nil {
		return err
	}
	units := string(tok)
	switch units {
	case "ns", "", "b":
		// already in nanoseconds or bytes
	case "us":
		x *= 1e3
	case "ms":
		x *= 1e6
	case "s":
		x *= 1e9
	case "k", "kb", "K", "KB":
		x *= 1024
	case "m", "mb", "M", "MB":
		x *= 1024 * 1024
	default:
		return fmt.Errorf("unknown time or size unit %q", units)
	}
	*u = unit(x)
	return nil
}

func nextRune(rd io.RuneReader) (r rune, err error) {
	for {
		r, _, err = rd.ReadRune()
		if err != nil {
			return
		}
		if !unicode.IsSpace(r) {
			return
		}
	}
	return
}
