package basic

import (
	"code.google.com/p/rog-go/exp/abc"
	"os"
)

func init() {
	abc.Register("stdout", map[string]abc.Socket{
		"1": abc.Socket{FdT, abc.Female},
	}, makeStdout)
}

func makeStdout(_ *abc.Status, args map[string]interface{}) abc.Widget {
	in := args["1"].(Fd)
	in.PutWriter(os.Stdout)
	return nil
}
