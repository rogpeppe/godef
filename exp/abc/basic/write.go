package basic
import (
	"abc"
	"os"
)

func init() {
	abc.Register("write", map[string]abc.Socket {
			"1": abc.Socket{FdT, abc.Female},
			"2": abc.Socket{abc.StringT, abc.Female},
		}, makeWrite)
}

func makeWrite(args map[string] interface{}) abc.Widget {
	in := args["1"].(Fd)
	f := args["2"].(string)

	fd, _ := os.Open(f, os.O_RDONLY | os.O_CREATE, 0644)
	in.PutWriter(fd)
	return nil
}

