import (
	"rog-go.googlecode.com/hg/exp/abc"
)

func init() {
	abc.Register("output", map[string]abc.Socket {
			"audio", abc.Socket{SamplesT, abc.Female},
			"1": abc.Socket{SamplesT, abc.Female},
			"2": abc.Socket{abc.StringT, abc.Female},
		}, makeOutput)
}

func makeOutput(args map[string] interface{}) abc.Widget {
	name := args["2"].(string)
	ctxt := args["audio"].(*context)
	if len(name) > 4 && name[0:4] == "buf." {
		look up buffer - if it's already there and populated
		then we've got an error.
		if not there are all, then we create it
	}else{
		error - we don't have any output devices...
	}
}

for{
	select{
	case msg := <-ctl:

	case renderreq := <-intermittent:
