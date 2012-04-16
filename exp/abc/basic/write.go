package basic

import (
	"code.google.com/p/rog-go/exp/abc"
	"os"
)

func init() {
	abc.Register("write", map[string]abc.Socket{
		"1": abc.Socket{FdT, abc.Female},
		"2": abc.Socket{abc.StringT, abc.Female},
	}, makeWrite)
}

func makeWrite(_ *abc.Status, args map[string]interface{}) abc.Widget {
	in := args["1"].(Fd)
	f := args["2"].(string)

	fd, _ := os.Create(f)
	in.PutWriter(fd)
	return nil
}
