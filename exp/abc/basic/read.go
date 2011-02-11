package basic
import (
	"abc"
	"io"
	"os"
)

func init() {
	abc.Register("read", map[string]abc.Socket {
			"1": abc.Socket{abc.StringT, abc.Female},
			"out": abc.Socket{FdT, abc.Male},
		}, makeRead)
}

func makeRead(args map[string] interface{}) abc.Widget {
	f := args["1"].(string)
	out := NewFd()
	args["out"].(chan interface{}) <- out
	fd, _ := os.Open(f, os.O_RDONLY, 0)
	if w := out.GetWriter(fd); w != nil {
		io.Copy(w, fd)
	}
	return nil
}
