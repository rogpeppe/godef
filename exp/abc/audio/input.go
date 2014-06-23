package audio

import (
	"code.google.com/p/rog-go/exp/abc"
)

func init() {
	abc.Register("input", map[string]abc.Socket{
		"audio", abc.Socket{SamplesT, abc.Female},
		"out": abc.Socket{SamplesT, abc.Male},
		"1":   abc.Socket{abc.StringT, abc.Female},
	}, makeInput)
}

func makeInput(args map[string]interface{}) abc.Widget {
	name := args["1"].(string)
	ctxt := args["audio"].(*context)
	ctxt.Lock()
	defer ctxt.Unlock()
	node := ctxt.inputs[name]
	if node == nil {
		if len(name) > 4 && name[0:4] == "buf." {
			ctxt.inputs[name] = &node{nullRender, nil}
		}
	}
	args["out"].(chan interface{}) <- node
	return nil
}
