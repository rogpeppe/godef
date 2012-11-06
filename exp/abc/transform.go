package abc

import (
	"container/vector"
	"fmt"
	"strconv"
)

// transform ADT into a set of commands

type tContext struct {
	*context
	cmds *vector.Vector
}

func (ctxt *context) transform(a *assign, cmds *vector.Vector) {
	tctxt := &tContext{context: ctxt, cmds: cmds}
	tctxt.transformAssign(a)
	fmt.Println("done transform")
}

func (ctxt *tContext) gensym() string {
	name := fmt.Sprint("_gen", ctxt.sym)
	ctxt.sym++
	return name
}

func (ctxt *tContext) transformAssign(a *assign) {
	fmt.Printf("transformAssign %v=%v\n", a.name, a.input)
	ctxt.transformPipe(a.input, a.name)
}

func (ctxt *tContext) transformPipe(p *pipe, out string) {
	fmt.Println("transformPipe", p, out)
	// a x | b y ->
	//	a x -out $>g
	//	b $g y

	// a x | b y | c z 
	//	a x -out $>g1
	//	b $g1 y -out $>g2
	//	c $g2 z
	if p == nil {
		if out != "" {
			panic("pipe with no input")
		}
		return
	}

	if p.input == nil {
		ctxt.transformUnit(p.unit, "", out)
	} else {
		id := ctxt.gensym()
		ctxt.transformPipe(p.input, id)
		ctxt.transformUnit(p.unit, id, out)
	}
}

func (ctxt *tContext) transformUnit(u *unit, in, out string) {
	fmt.Printf("transformUnit %#v %#v %#v\n", u, in, out)
	// a x {b y} z ->
	// b y -out $>g
	// a x $g z

	// what about:
	// a -out foo | b
	// ?

	c := &command{name: u.name, conns: make(map[string]*endpoint)}
	args := u.args

	if in != "" {
		args = &arg{
			typ:    aEndpoint,
			value:  in,
			gender: Female,
			next:   args,
		}
	}
	if out != "" {
		args = &arg{
			typ:     aEndpoint,
			argname: "out",
			value:   out,
			gender:  Male,
			next:    args,
		}
	}
	ctxt.transformArgs(args, c, 1)
	ctxt.cmds.Push(c)
}

func (ctxt *tContext) transformArgs(a *arg, c *command, n int) {
tailcall:
	fmt.Printf("transformArg %d %#v\n", n, a)
	if a == nil {
		return
	}
	var e *endpoint
	argname := a.argname
	if argname == "" {
		argname = strconv.Itoa(n)
		n++
	}
	switch a.typ {
	case aString:
		e = ctxt.stringEndpoint(a.value, c)

	case aQstring:
		text, err := strconv.Unquote(a.value)
		if err != nil {
			panic("strunquote failed on: " + a.value)
		}
		e = ctxt.stringEndpoint(text, c)

	case aBlock:
		g := ctxt.gensym()
		ctxt.transformPipe(a.block, g)
		e = ctxt.endpoint(g, Female, c)

	case aEndpoint:
		e = ctxt.endpoint(a.value, a.gender, c)

	default:
		panic("unknown arg type")
	}

	if c.conns[argname] != nil {
		panic("arg already set: " + argname)
	}
	c.conns[argname] = e
	a = a.next
	goto tailcall
}
