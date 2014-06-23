package audio

import (
	"code.google.com/p/rog-go/exp/abc"
	"fmt"
	"strings"
	"sync"
)

var SamplesT = &abc.Type{"samples", false, abc.IsType((*node)(nil))}
var AudioEnvT = &abc.Type{"audio", true, abc.IsType((*context)(nil))}
var TimeT = &abc.Type{"time", true, abc.IsType(Time{})}

type widgetKind int

const (
	wInput = widgetKind(iota)
	wOutput
	wProc
	wConverter // converter widget not part of original network
	wBuffer
)

type auWidget struct {
	name    string
	argtype map[string]abc.Socket
	kind    widgetKind

	// the make function creates the widget, but
	// all its audio inputs will be uninitialised until
	// Init is called.
	make func(status *abc.Status, args map[string]interface{}) Widget
}

type Widget interface {
	Formatted
	Init(inputs map[string]Widget)
	ReadSamples(buf Buffer, t int64) bool
}

type context struct {
	sync.Mutex
	defaultFormat Format
	inputs        map[string]*node
}

type node struct {
	doneInit bool
	kind     widgetKind
	ctxt     *context
	inputs   map[string]*node
	buffer   *node // buffer that this node is a reader for.
	w        Widget
}

func init() {
	abc.Register("buf", map[string]abc.Socket{
		"1": abc.Socket{SamplesT, abc.Female},
		"2": abc.Socket{abc.StringT, abc.Female},
	},
		makeBuffer)

	abc.Register("input", map[string]abc.Socket{
		"out": abc.Socket{SamplesT, abc.Male},
		"env": abc.Socket{AudioEnvT, abc.Female},
		"1":   abc.Socket{abc.StringT, abc.Female},
	},
		makeInput)

	abc.Register("audioenv", map[string]abc.Socket{
		"out": abc.Socket{AudioEnvT, abc.Male},
	},
		makeAudioenv)
}

func Register(name string, kind widgetKind, argtype map[string]abc.Socket, fn func(status *abc.Status, args map[string]interface{}) Widget) {
	auw := &auWidget{
		name:    name,
		kind:    kind,
		argtype: argtype,
		make:    fn,
	}
	abc.Register(name, argtype, func(status *abc.Status, args map[string]interface{}) abc.Widget {
		return makeAudioWidget(auw, status, args)
	})
}

func makeAudioenv(status *abc.Status, args map[string]interface{}) abc.Widget {
	args["out"].(chan interface{}) <- &context{inputs: make(map[string]*node)}
	return nil
}

func makeAudioWidget(auw *auWidget, status *abc.Status, args map[string]interface{}) abc.Widget {
	n := &node{}
	n.inputs = make(map[string]*node, len(args))
	n.w = auw.make(status, args)
	var outc chan interface{}
	for opt, arg := range args {
		if t := auw.argtype[opt]; t.Gender == abc.Female {
			var ctxt *context
			switch t.Type {
			case SamplesT:
				input := arg.(*node)
				ctxt = input.ctxt
				n.inputs[opt] = input
			case AudioEnvT:
				ctxt = arg.(*context)
			}
			if ctxt != nil {
				if n.ctxt == nil {
					n.ctxt = ctxt
				} else if n.ctxt != ctxt {
					panic("mismatched audio context")
				}
			}
		} else if t.Type == SamplesT && opt == "out" {
			outc = arg.(chan interface{})
		}
	}
	if outc == nil {
		fireEmUp(n)
	} else {
		outc <- n
	}

	return nil
}

func makeInput(log *abc.Status, args map[string]interface{}) abc.Widget {
	ctxt := args["env"].(*context)
	name := args["1"].(string)
	ctxt.Lock()
	defer ctxt.Unlock()
	args["out"].(chan interface{}) <- getInput(ctxt, name)
	return nil
}

func makeBuffer(status *abc.Status, args map[string]interface{}) abc.Widget {
	input := args["1"].(*node)
	name := args["2"].(string)
	if !strings.HasPrefix(name, "buf.") {
		panic("buffer name must start with 'buf.'")
	}
	ctxt := input.ctxt
	ctxt.Lock()
	defer ctxt.Unlock()

	buf := ctxt.inputs[name]
	if buf != nil && buf.inputs != nil {
		panic("buffer already created")
	}
	args["out"].(chan interface{}) <- getInput(ctxt, name)
	buf.inputs = map[string]*node{"1": input}
	return nil
}

func getInput(ctxt *context, name string) *node {
	buf := ctxt.inputs[name]
	if buf == nil {
		if strings.HasPrefix(name, "buf.") {
			// forward reference to buffer widget triggers its creation
			buf = &node{
				kind: wProc,
				ctxt: ctxt,
				w:    RingBuf(nil, 0, 0, 0),
			}
			ctxt.inputs[name] = buf
		} else {
			panic("unknown input")
		}
	}
	return &node{
		kind:   wBuffer,
		ctxt:   ctxt,
		buffer: buf,
		w:      buf.w.(*RingBufWidget).Reader(false, 0),
	}
}

var defaultFormat = Format{
	Layout:   Interleaved,
	Type:     Float32Type,
	Rate:     44100,
	NumChans: 2,
}

func fireEmUp(n *node) {
	if !initNodes(n, make(map[*node]bool), Format{}) {
		deflt := defaultFormat

		if n.ctxt != nil {
			deflt = n.ctxt.defaultFormat
		}
		if !initNodes(n, make(map[*node]bool), deflt) {
			panic("failed to initialise nodes")
		}
	}
}

// initNodes initialises n and, recursively, all the nodes
// that it points to. deflt gives a suggestion as to a possible
// output format - if it is fully specified, then the initialisation
// must take place; if not, then initNodes may return false
// to indicate that no input with a fully specified format was
// found.
//
func initNodes(n *node, visiting map[*node]bool, deflt Format) (ok bool) {
	defer un(log("initNodes %#v", n), &ok)
	if n.doneInit {
		return true
	}
	if n.kind == wBuffer && !n.buffer.doneInit && !visiting[n.buffer] {
		visiting[n.buffer] = true
		ok := initNodes(n.buffer, visiting, deflt)
		visiting[n.buffer] = false
		if !ok {
			return false
		}
	}
	if len(n.inputs) > 0 {
		var best Format
		again := make(map[string]*node, len(n.inputs))
		for name, input := range n.inputs {
			f := input.w.GetFormat(name).Combine(deflt)
			if !initNodes(input, visiting, f) {
				again[name] = input
			} else {
				f := input.w.GetFormat("out")
				if !f.FullySpecified() {
					panic("output format not fully specified")
				}
				best = best.CombineBest(f)
			}
		}
		// some inputs hadn't specified their format, so redo those inputs
		// with a fully specified format.
		if len(again) > 0 {
			if !best.FullySpecified() {
				return false
			}
			for name, input := range again {
				f := input.w.GetFormat(name).Combine(best)
				if !initNodes(input, visiting, f) {
					panic("node initialisation failed")
				}
			}
		}

		// all inputs should now have fully specified formats.
		// insert converter widgets wherever they don't have
		// the required format.
		for name, input := range n.inputs {
			f0 := n.w.GetFormat(name)
			f1 := input.w.GetFormat("out")
			if !f1.FullySpecified() {
				panic(fmt.Sprintf("output format of %T not fully specified (%#v)", input.w, f1))
			}
			if !f0.Match(f1) {
				f0 = f0.Combine(f1)
				n.inputs[name] = &node{
					kind:   wConverter,
					ctxt:   n.ctxt,
					inputs: map[string]*node{"0": input},
					w:      Converter(input.w, f0),
				}
			}
		}
	} else if !n.w.GetFormat("out").FullySpecified() {
		if w, ok := n.w.(FormatSetter); ok {
			// if there are no inputs, set the nodes format
			// from the default format if necessary
			if nf := n.w.GetFormat("out"); !nf.FullySpecified() {
				nf = nf.Combine(deflt)
				if !nf.FullySpecified() {
					return false // XXX some parts of format are lost.
				}
				w.SetFormat(nf)
			} else {
				panic(fmt.Sprintf("no inputs, non-fully-specified format (%#v), and cannot set format, %T", n.w))
			}
		}
	}

	ws := make(map[string]Widget, len(n.inputs))
	for name, input := range n.inputs {
		w := input.w
		if !w.GetFormat("out").FullySpecified() {
			panic(fmt.Sprintf("input format %T not fully specified; format %#v", w, w.GetFormat("out")))
		}
		ws[name] = w
	}
	n.w.Init(ws)
	return true
}
