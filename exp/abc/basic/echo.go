package basic
import (
	"abc"
	"strings"
)

func init() {
	abc.Register("echo", map[string]abc.Socket {
			"1": abc.Socket{abc.StringT, abc.Female},
			"out": abc.Socket{FdT, abc.Male},
		}, makeEcho)
}

func makeEcho(args map[string] interface{}) abc.Widget {
	s := args["1"].(string) + "\n"
	out := NewFd()
	args["out"].(chan interface{}) <- out
	if w := out.GetWriter(strings.NewReader(s)); w != nil {
		w.Write([]byte(s))
	}
	return nil
}
