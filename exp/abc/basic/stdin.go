package basic
import (
	"abc"
	"io"
	"os"
)

func init() {
	abc.Register("stdin", map[string]abc.Socket {
			"out": abc.Socket{FdT, abc.Male},
		}, makeStdin)
}

func makeStdin(args map[string] interface{}) abc.Widget {
	out := NewFd()
	args["out"].(chan interface{}) <- out
	if w := out.GetWriter(os.Stdin); w != nil {
		io.Copy(w, os.Stdin)
	}
	return nil
}
