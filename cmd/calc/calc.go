package main

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"strings"
)

// TODO:
// 	testing
//	parse type[value]
//	formatting with /x, /%.5d etc

const debug = false

type genericOp struct {
	numIn, numOut int
	f             func(s *stack, name string)
}

var errStackUnderflow = errors.New("stack underflow")

func main() {
	// sanity check ops table
	for _, vs := range ops {
		for _, v := range vs {
			argCount(v)
		}
	}
	s := &stack{format: "%v"}
	arg, _ := parseArgs(os.Args[1:], "")
	if debug {
		fmt.Printf("parsed %v\n", words(arg))
	}
	s.interp(arg)
	for _, v := range s.items {
		fmt.Println(v)
	}
}

type op struct {
	name string
	body []op
}

func (o op) String() string {
	switch o.name {
	case "[":
		return fmt.Sprintf("[ %s ]", words(o.body))
	case "[[":
		return fmt.Sprintf("[[ %s ]]", words(o.body))
	}
	return o.name
}

func words(arg []op) string {
	if len(arg) == 0 {
		return ""
	}
	b := []byte(arg[0].String())
	for _, a := range arg[1:] {
		b = append(b, ' ')
		b = append(b, a.String()...)
	}
	return string(b)
}

func parseArgs(arg []string, end string) (ops []op, rest []string) {
	for len(arg) > 0 {
		o := op{name: arg[0]}
		switch o.name {
		case "[":
			o.body, arg = parseArgs(arg[1:], "]")
		case "[[":
			o.body, arg = parseArgs(arg[1:], "]]")
		case "]", "]]":
			if o.name != end {
				fatalf("unexpected %q", o.name)
			}
			return ops, arg[1:]
		default:
			arg = arg[1:]
		}
		ops = append(ops, o)
	}
	if end != "" {
		fatalf("expected %q but did not find it", end)
	}
	return ops, arg
}

var bigOne = big.NewInt(1)

type stack struct {
	format    string
	minDepth  int
	items     []value
	callDepth int
}

func (s *stack) logf(f string, a ...interface{}) {
	if !debug {
		return
	}
	if f[len(f)-1] == '\n' {
		f = f[0 : len(f)-1]
	}
	fmt.Printf("%s%s\n", strings.Repeat("\t", s.callDepth), fmt.Sprintf(f, a...))
}

func (s *stack) push(old value, new interface{}) {
	s.items = append(s.items, value{v: new, format: old.format})
}

func (s *stack) pushX(new interface{}) {
	s.items = append(s.items, value{
		format: s.format,
		v:      new,
	})
}

func (s *stack) pop() value {
	return s.popN(1)[0]
}

func (s *stack) popN(n int) []value {
	d := len(s.items) - n
	if d < 0 {
		panic(errStackUnderflow)
	}
	v := s.items[d:]
	s.items = s.items[0:d]
	if d < s.minDepth {
		s.minDepth = d
	}
	return v
}

func (s *stack) interp(arg []op) {
	for _, a := range arg {
		s.logf("interp %v\n", a)
		switch a.name {
		case "[":
			s.reduce(a)
		case "[[":
			s.repeat(a)
		default:
			s.interp1(a.name)
		}
		s.logf("-- (min %d) %v\n", s.minDepth, s.items)
	}
}

func (s *stack) interp1(a string) {
	// operators and constants
	if vs := ops[a]; vs != nil {
		s.exec(a, vs)
		return
	}
	// aliases
	if b := alias[a]; b != "" {
		s.exec(a, ops[b])
		return
	}
	// patterns
	for _, p := range patterns {
		if p.pat.MatchString(a) {
			s.exec(a, []interface{}{p.f})
			return
		}
	}
	s.fatalf("unknown argument %q", a)
}

func (s *stack) exec(name string, vs []interface{}) {
	defer func() {
		e := recover()
		if e == nil {
			return
		}
		if e == errStackUnderflow {
			fatalf("stack underflow on %q", name)
		}
		panic(e)
	}()
	// Check whether operator is a constant or is generic, in which case we just call it.
	switch v := vs[0].(type) {
	case *big.Int, *big.Rat, float64:
		s.pushX(v)
		return
	case genericOp:
		v.f(s, name)
		return
	}

	arg := s.popN(argCount(vs[0]))
	f := vs[0]
	if len(vs) > 0 {
		argt := reflect.TypeOf(arg[0].v)
		// The first argument determines the operator to use if there's
		// more than one, but we default to the first operator if
		// none matches.
		for _, v := range vs {
			if reflect.TypeOf(v).In(0) == argt {
				f = v
				break
			}
		}
	}
	switch f := f.(type) {
	case func(*big.Int, *big.Int) *big.Int:
		r := arg[0].toInt()
		s.push(arg[0], f(r, r))
	case func(*big.Int, *big.Int, *big.Int) *big.Int:
		r := arg[0].toInt()
		s.push(arg[0], f(r, r, arg[1].toInt()))
	case func(*big.Int, *big.Int, *big.Int, *big.Int) *big.Int:
		r := arg[0].toInt()
		s.push(arg[0], f(r, r, arg[1].toInt(), arg[2].toInt()))
	case func(*big.Rat, *big.Rat) *big.Rat:
		r := arg[0].toRat()
		s.push(arg[0], f(r, r))
	case func(*big.Rat, *big.Rat, *big.Rat) *big.Rat:
		r := arg[0].toRat()
		s.push(arg[0], f(r, r, arg[1].toRat()))
	case func(float64) float64:
		s.push(arg[0], f(arg[0].toFloat()))
	case func(float64, float64) float64:
		s.push(arg[0], f(arg[0].toFloat(), arg[1].toFloat()))
	default:
		panic(fmt.Errorf("unrecognised function type %T", f))
	}
}

// argCount returns the number of arguments consumed
// by a function. It also checks that the function is well formed.
func argCount(f interface{}) int {
	t := reflect.TypeOf(f)
	switch f := f.(type) {
	case *big.Int, *big.Rat, float64:
		return 0
	case genericOp:
		return f.numIn
	}
	n := t.NumIn()
	rcvr := t.In(0)
	if rcvr.Kind() == reflect.Ptr {
		// The first argument of methods of *big.Int and *big.Rat is the receiver.
		n--
	}
	for i := t.NumOut() - 1; i >= 0; i-- {
		if t.Out(i) != rcvr {
			printAll()
			panic(fmt.Errorf("unexpected return value in %T", f))
		}
	}
	for i := t.NumOut() - 1; i >= 1; i-- {
		if t.Out(i) != rcvr {
			printAll()
			panic(fmt.Errorf("unexpected return value in %T", f))
		}
	}
	return n
}

func (s *stack) reduce(o op) {
	t := s.subexprStack()
	nin, nout := t.runOneSubexpr(o.body)
	if nout >= nin {
		s.fatalf("operations inside %v must reduce the size of the stack (nin %d, nout %d)", o, nin, nout)
	}

	for len(t.items) >= nin {
		t.interp(o.body)
	}
	s.items = append(s.items, t.items...)
}

func (s *stack) repeat(o op) {
	t := s.subexprStack()
	nin, nout := t.runOneSubexpr(o.body)
	if nin <= 0 {
		s.fatalf("%v uses no items from stack", o)
	}
	if (len(t.items)-nout)%nin != 0 {
		s.fatalf("extra items on stack after %v", o)
	}
	origOut := t.items[len(t.items)-nout:]
	origItems := t.items[0 : len(t.items)-nout]
	t.items = nil
	for i := 0; i < len(origItems); i += nin {
		t.items = append(t.items, origItems[i:i+nin]...)
		t.interp(o.body)
	}
	// N.B. append origOut to t.items before appending to s.items
	// because origOut aliases s.items.
	t.items = append(t.items, origOut...)
	s.items = append(s.items, t.items...)
}

// subexprStack makes a partial clone of s to be used
// for executing a subexpression, The new stack
// holds values from the minimum used depth (i.e. values used by the
// enclosing [] or [[]] block). It pops all those
// values off s, to be replaced later.
func (s *stack) subexprStack() *stack {
	t := *s
	t.items = t.items[s.minDepth:]
	t.callDepth++
	s.items = s.items[0:s.minDepth]
	s.minDepth = len(s.items)
	return &t
}

// runOneSubexpr runs a subexpression once
// to determine its input and output count.
func (s *stack) runOneSubexpr(arg []op) (nin, nout int) {
	s.minDepth = len(s.items)
	origDepth := len(s.items)
	s.interp(arg)
	nin = origDepth - s.minDepth
	nout = len(s.items) - s.minDepth
	return
}

type value struct {
	v      interface{}
	format string
}

func (v value) Format(f fmt.State, c rune) {
	if debug || f.Flag('#') {
		fmt.Fprintf(f, "%s["+v.format+"]", typeName(v.v), v.v)
	} else {
		fmt.Fprintf(f, v.format, v.v)
	}
}

func typeName(x interface{}) string {
	switch x.(type) {
	case *big.Int:
		return "int"
	case *big.Rat:
		return "rat"
	case float64:
		return "float"
	}
	return fmt.Sprintf("%T", x)
}

func (v value) toInt() *big.Int {
	switch v := v.v.(type) {
	case *big.Int:
		return v
	case *big.Rat:
		// TODO rounding?
		return new(big.Int).Quo(v.Num(), v.Denom())
	case float64:
		// TODO convert numbers out of the range of int64 (use math.Frexp?)
		return big.NewInt(int64(v))
	}
	panic(fmt.Errorf("unexpected type %T", v.v))
}

func (v value) toFloat() float64 {
	switch v := v.v.(type) {
	case *big.Int:
		// TODO convert numbers out of the range of int64 (use math.Frexp?)
		return float64(v.Int64())
	case *big.Rat:
		// TODO better
		return float64(v.Num().Int64()) / float64(v.Denom().Int64())
	case float64:
		return v
	}
	panic(fmt.Errorf("unexpected type %T", v.v))
}

func (v value) toRat() *big.Rat {
	switch v := v.v.(type) {
	case *big.Int:
		return new(big.Rat).SetFrac(v, bigOne)
	case *big.Rat:
		return v
	case float64:
		// TODO efficiency
		r, ok := new(big.Rat).SetString(fmt.Sprint(v))
		if !ok {
			fatalf("cannot convert %v to rational", v)
		}
		return r
	}
	panic(fmt.Errorf("unexpected type %T", v.v))
}

func (s *stack) fatalf(format string, args ...interface{}) {
	format = strings.Repeat("\t", s.callDepth) + format
	fatalf(format, args...)
}

func fatalf(format string, args ...interface{}) {
	m := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "fc: %s\n", m)
	os.Exit(2)
}
