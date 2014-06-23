package audio

import (
	"code.google.com/p/rog-go/exp/abc"
	"fmt"
	"strconv"
	"strings"
	"os"
)

type Time struct {
	t    int64
	real bool // real time (in nanoseconds) ?
}

func init() {
	abc.Register("time", map[string]abc.Socket{
		"1":   abc.Socket{abc.StringT, abc.Female},
		"out": abc.Socket{TimeT, abc.Male},
	}, makeTime)
}

func makeTime(status *abc.Status, args map[string]interface{}) abc.Widget {
	s := args["1"].(string)
	t, err := ParseTime(s)
	if err != nil {
		panic(fmt.Sprintf("cannot make time from %#s: %s", s, err.String()))
	}
	args["out"].(chan interface{}) <- t
	return nil
}
func isDigit(r int) bool {
	return r >= 0 && r <= '9'
}

// ParseTime parses the time interval from s, and returns it
// as nanoseconds, if it is representable in seconds (with
// isNanoSeconds true), or sample count (with isNanoSeconds false)
// if not.
func ParseTime(s string) (t Time, err os.Error) {
	endi := strings.LastIndexFunc(s, isDigit) + 1
	if endi == 0 {
		return Time{}, os.NewError("invalid number")
	}
	number, suffix := s[0:endi], s[endi:]
	var mult int64
	switch suffix {
	case "s", "":
		mult = 1e9
	case "ms":
		mult = 1e6
	case "us":
		mult = 1e3
	case "ns":
		mult = 1
	case "x": // samples
		mult = 1
	}
	// use exact arithmetic if we can
	d, err := strconv.Atoi64(number)
	if err != nil {
		f, err := strconv.Atof64(number)
		if err != nil {
			return Time{}, err
		}
		d = int64(f * float64(mult))
	} else {
		d *= mult
	}
	return Time{d, suffix != "x"}, nil
}
