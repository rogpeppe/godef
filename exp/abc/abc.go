package abc

import (
	"container/vector"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"text/scanner"
)

type Type struct {
	Name string
	Mux  bool
	Test func(interface{}) bool
}

var StringT = &Type{
	"string",
	true,
	IsType(""),
}

type Gender bool

const (
	Male   = Gender(false)
	Female = Gender(true)
)

type Socket struct {
	*Type
	Gender Gender
}

type Widget interface {
	//	Type() map[string] Socket
	Plug(s string, w interface{})
}

type widgetFactory struct {
	Type map[string]Socket
	make func(log *Status, args map[string]interface{}) Widget
}

var widgets = make(map[string]widgetFactory)

func Register(name string, t map[string]Socket, fn func(status *Status, args map[string]interface{}) Widget) {
	fmt.Printf("registering %s\n", name)
	widgets[name] = widgetFactory{t, fn}
}

func Unregister(name string) {
	delete(widgets, name)
}

type context struct {
	idents map[string]*ident
	tok    *scanner.Scanner
	sym    int
}

type ident struct {
	Type     *Type
	male     *endpoint
	nfemales int
	wire     chan interface{}
}

type endpoint struct {
	name string
	Type *Type
	*ident
	cmd    *command
	gender Gender
}

type command struct {
	visit   bool
	name    string
	factory widgetFactory
	conns   map[string]*endpoint
}

func (c *command) String() string {
	if c == nil {
		return "<nil>"
	}
	return fmt.Sprintf("&command{%#v, %v}", c.name, c.conns)
}

func (e *endpoint) String() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("&endpoint{%#v, %#v, %v}", e.name, e.Type, e.gender)
}

var interpCmd command
var interpEndpoint = endpoint{cmd: &interpCmd}

func (e *endpoint) value() (x interface{}) {
	if e.gender == Male {
		x = e.wire
	} else {
		x = <-e.wire
		if !e.Type.Test(x) {
			panic("value of unexpected type")
		}
		e.wire <- x
	}
	return
}

// N.B. string is go-style double quoted
func (ctxt *context) strIdent(s string) *ident {
	if id, ok := ctxt.idents[s]; ok {
		return id
	}
	id := new(ident)
	ctxt.idents[s] = id
	id.Type = StringT
	return id
}

func Exec2(s string) {
	lex := new(abcLex).Init(strings.NewReader(s))
	var ctxt context
	ctxt.idents = make(map[string]*ident)
	var v vector.Vector
	for {
		a := lex.Parse()
		if a == nil {
			break
		}
		ctxt.transform(a, &v)
	}
	cmds := make([]*command, len(v))
	for i, c := range v {
		cmds[i] = c.(*command)
	}
	fmt.Printf("transformed to %v\n", cmds)
	ctxt.run(cmds)
}

func Exec(s string) {
	var ctxt context
	ctxt.idents = make(map[string]*ident)
	ctxt.tok = new(scanner.Scanner).Init(strings.NewReader(s))
	ctxt.tok.Whitespace &^= 1 << '\n'

	cmds := ctxt.readCommands()
	if len(cmds) == 0 {
		return
	}
	fmt.Printf("read %d commands\n", len(cmds))
	ctxt.run(cmds)
}

func (ctxt *context) run(cmds []*command) {
	// check types
	for _, c := range cmds {
		fmt.Printf("cmd %s\n", c.name)
		ctxt.checkType(c)
	}

	// check type usage
	for name, id := range ctxt.idents {
		fmt.Printf("check id %s\n", name)
		if id.male == nil {
			ctxt.fail("'%s' has no producer", name)
		}
		if !id.Type.Mux && id.nfemales != 1 {
			ctxt.fail("'%s' of type '%s' requires exactly one consumer", name, id.Type.Name)
		}
	}

	var path vector.StringVector
	// check for cyclic dependency
	ctxt.checkCyclic(cmds[0], &path)

	m := new(StatusManager)

	sync := make(chan bool)
	for _, c := range cmds {
		m.Go(func(status *Status) {
			ctxt.startWidget(c, status, sync)
		})
		<-sync
	}

	m.Wait()
}

func (ctxt *context) startWidget(c *command, status *Status, sync chan bool) {
	args := make(map[string]interface{})
	// non-muxable types get transferred synchronously
	for name, e := range c.conns {
		if e.Type.Mux {
			args[name] = e.value()
		}
	}
	sync <- true
	for name, e := range c.conns {
		if !e.Type.Mux {
			args[name] = e.value()
		}
	}
	c.factory.make(status, args)
}

func (ctxt *context) checkCyclic(c *command, path *vector.StringVector) {
	fmt.Printf("checkCyclic %s\n", c.name)
	if c.visit {
		ctxt.fail("cyclic dependency at %s, path %v", c.name, path)
	}
	c.visit = true
	path.Push(c.name)
	for _, e := range c.conns {
		if e.gender == Female {
			ctxt.checkCyclic(e.male.cmd, path)
		}
	}
	path.Pop()
	c.visit = false
}

func (ctxt *context) checkType(c *command) {
	wf, ok := widgets[c.name]
	if !ok {
		ctxt.fail("cannot find widget '%s'", c.name)
	}
	wt := wf.Type
	for argname, e := range c.conns {
		sock, ok := wt[argname]
		if !ok {
			if argname[0] >= '1' && argname[0] <= '9' {
				sock, ok = wt["*"]
			}
			if !ok {
				ctxt.fail("unknown argument '%s' on '%s'", argname, c.name)
			}
		}
		if e.gender != sock.Gender {
			ctxt.fail("mismatched gender, argument '%s' on '%s'", argname, c.name)
		}
		fmt.Printf("e.ident: %v\n", e.ident)
		if e.ident.Type != nil && e.ident.Type != sock.Type {
			ctxt.fail("mismatched type, argument '%s' on '%s'", argname, c.name)
		}
		e.ident.Type = sock.Type
		e.Type = sock.Type
	}
	c.factory = wf
}

func (ctxt *context) readCommands() (cmds []*command) {
	var v vector.Vector
	for {
		c := ctxt.parseLine()
		if c == nil {
			break
		}
		v.Push(c)
	}
	cmds = make([]*command, len(v))
	for i, c := range v {
		cmds[i] = c.(*command)
	}
	return
}

func (ctxt *context) parseLine() (c *command) {
	tok := ctxt.tok
	t := tok.Scan()
	for t == '\n' {
		t = tok.Scan()
	}
	if t == scanner.EOF {
		return
	}
	c = new(command)
	c.conns = make(map[string]*endpoint)
	if t != scanner.Ident {
		fail(tok, "expected command name, got %d (%s)", t, tok.TokenText())
	}
	c.name = tok.TokenText()
	fmt.Printf("parsing cmd %s\n", c.name)
	i := 1
	for {
		argname := ""
		t := tok.Scan()
		fmt.Printf("arg %s\n", toktext(t))
		if t == '-' {
			if tok.Scan() != scanner.Ident {
				fail(tok, "expected option name after '-'")
			}
			argname = tok.TokenText()
			t = tok.Scan()
		} else {
			argname = strconv.Itoa(i)
			i++
		}
		var e *endpoint
		text := tok.TokenText()
		switch t {
		case '$':
			gender := Female
			t = tok.Scan()
			if t == '>' {
				gender = Male
				t = tok.Scan()
			}
			if t != scanner.Ident {
				fail(tok, "expected identifier")
			}
			e = ctxt.endpoint(tok.TokenText(), gender, c)

		case scanner.String, scanner.RawString, scanner.Char:
			text, _ = strconv.Unquote(text)
			fallthrough

		case scanner.Ident, scanner.Float, scanner.Int:
			e = ctxt.stringEndpoint(text, c)

		case '\n', scanner.EOF:
			return

		default:
			panic(fmt.Sprintf("unknown endpoint type %d", t))
		}
		c.conns[argname] = e
	}
	return
}

func (ctxt *context) endpoint(name string, gender Gender, c *command) *endpoint {
	e := &endpoint{name: name, cmd: c, gender: gender}
	id, ok := ctxt.idents[name]
	if !ok {
		id = &ident{wire: make(chan interface{}, 1)}
		ctxt.idents[name] = id
	}
	if e.gender == Male {
		if id.male != nil {
			ctxt.fail("multiple producers of identifer %q", name)
		}
		id.male = e
	} else {
		id.nfemales++
	}
	e.ident = id
	return e
}

func (ctxt *context) stringEndpoint(text string, c *command) *endpoint {
	name := strconv.Quote(text)
	e := &endpoint{name: name, cmd: c, gender: Female}
	id, ok := ctxt.idents[name]
	if !ok {
		id = &ident{wire: make(chan interface{}, 1)}
		id.male = &interpEndpoint
		id.Type = StringT
		id.wire <- text
		ctxt.idents[name] = id
	}

	e.ident = id
	id.nfemales++
	return e
}

func (ctxt *context) fail(f string, a ...interface{}) {
	fmt.Println("error: " + fmt.Sprintf(f, a...))
	os.Exit(2)
}

func fail(tok *scanner.Scanner, f string, a ...interface{}) {
	fmt.Println(tok.String(), ": ", fmt.Sprintf(f, a...))
	os.Exit(2)
}

func toktext(t int) string {
	switch t {
	case scanner.EOF:
		return "EOF"
	case scanner.Ident:
		return "Ident"
	case scanner.Int:
		return "Int"
	case scanner.Float:
		return "Float"
	case scanner.Char:
		return "Char"
	case scanner.String:
		return "String"
	case scanner.RawString:
		return "RawString"
	case scanner.Comment:
		return "Comment"
	}
	return "'" + string(t) + "'"
}

func IsType(t interface{}) func(x interface{}) bool {
	typ := reflect.TypeOf(t)
	return func(x interface{}) bool {
		return reflect.TypeOf(x) == typ
	}
}
