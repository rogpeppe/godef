package loopback
import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

type errEmpty int

func (errEmpty) String() string {
	return "empty"
}

func parseNetwork(net string) (inOpts, outOpts Options, actualNet string, err os.Error) {
	if net == "" {
		err = os.ErrorString("empty network name")
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
			if err == errEmpty(0) {
				err = nil
				return
			}
			break
		}

		var r int
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

type opts struct {
	inOpts *Options
	outOpts *Options
}

func (o opts) Scan(state fmt.ScanState, verb int) os.Error {
	var attr string
	var t int64
	if _, err := fmt.Fscanf(state, "%v=%v", (*word)(&attr), (*unit)(&t)); err != nil {
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
	if err := setOpt(in, attr, t); err != nil {
		return err
	}
	if err := setOpt(out, attr, t); err != nil {
		return err
	}
	return nil
}
	
func setOpt(opt *Options, attr string, t int64) os.Error {
	if opt == nil {
		return nil
	}
	switch attr {
	case "latency":
		opt.Latency = t
	case "bytedelay":
		opt.ByteDelay = t
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

type word string

func (w *word) Scan(state fmt.ScanState, verb int) os.Error {
	tok, err := state.Token(false, func(r int)bool{return unicode.IsLetter(r) || r == '.'})
	if err == nil && len(tok) == 0 {
		return errEmpty(0)
	}
	*w = word(tok)
	return err
}

type unit int64

func (u *unit) Scan(state fmt.ScanState, verb int) os.Error {
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
		x *= 1024*1024
	default:
		return fmt.Errorf("unknown time or size unit %q", units)
	}
	*u = unit(x)
	return nil
}

func nextRune(rd io.RuneReader) (r int, err os.Error) {
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
