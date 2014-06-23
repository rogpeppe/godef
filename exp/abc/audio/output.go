// +build ignore

package audio

import (
	"code.google.com/p/rog-go/exp/abc"
)

func init() {
	abc.Register("output", map[string]abc.Socket{
		"audio", abc.Socket{SamplesT, abc.Female},
		"1": abc.Socket{SamplesT, abc.Female},
		"2": abc.Socket{abc.StringT, abc.Female},
	}, makeOutput)
}

func makeOutput(args map[string]interface{}) abc.Widget {
	name := args["2"].(string)
	ctxt := args["audio"].(*context)
	if len(name) > 4 && name[0:4] == "buf." {
		look
		if it {
			then
			if not {
				are

				for {
					select {
					case msg := <-ctl:

					case renderreq := <-intermittent:
					}
				}
			}
		}
	}
}
