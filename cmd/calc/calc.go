package main

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// TODO:
// 	testing
//	subexpressions
//	parse type[value]
//	formatting with /x, /%.5d etc


func subexpr(a []string, stop string) (expr, rest []string) {
	start := a[0]
	depth := 1
	for i, s := range a {
		switch s {
		case start:
			depth++
		case stop:
			if depth--; depth == 0 {
				return a[1:i], a[i+1:]
			}
		}
	}
	fatalf("mismatched " + start)
	panic("not reached")
}

//sub-expressions:
//
//	[ operations ]
//		operations must reduce the size of the stack.
//		they're repeated until there are no elements in the stack.
//	[[ operations ]]
//		operations are repeated for each set of items that
//		hasn't been touched by the previous operations.
//
//with both forms, it's an error if there are items left untouched.
//
//so:
//
//fc 2 3 4 5 [ x ]
//-> 24
//
//fc 2 [ x ]
//-> error
//
//fc 2 3 4 5 [[ x ]]
//-> 6 20
//
//fc 2 3 4 [[ x ]]
//-> error
//
//executes operations repeatedly.
//if the stack size goes down, just do repeatedly.
//if the stack size goes up, it's an error.
//if the stack size stays the same, repeat for items left in stack that haven't been touched.
//
//for sub-expressions, execute once; if there's a 'stack too empty' error, ignore it.

type pattern struct {
	pat *regexp.Regexp
	f   func(s *stack, p string)
}

const floatPat = `(([0-9]+(\.[0-9]+)?)|([0-9]*(\.[0-9]+)))([eE]-?[0-9]+)?`

var patterns = []pattern{
	// format specifier
	{
		regex("%.*[fgvexXoObB]"),
		func(s *stack, p string) {
			// % has a special case - if the stack is empty, it sets
			// the default format output.
			if len(s.items) == 0 {
				s.format = p
			} else {
				v := s.pop()
				v.format = p
				s.push(v, v.v)
			}
		},
	},
	// integer literal
	{
		regex("0[bB][01]+|0[xX][0-9a-fA-F]+|0[0-9]+|[0-9]+"),
		func(s *stack, p string) {
			i, ok := new(big.Int).SetString(p, 0)
			if !ok {
				fatalf("cannot convert %q to int", p)
			}
			s.pushX(i)
		},
	},
	// rational literal
	{
		regex(floatPat + "/" + floatPat),
		func(s *stack, p string) {
			i := strings.Index(p, "/")
			r0, ok0 := new(big.Rat).SetString(p[0 : i])
			r1, ok1 := new(big.Rat).SetString(p[i+1:])
			if !ok0 || !ok1 {
				fatalf("bad rational %q", p)
			}
			s.pushX(r0.Quo(r0, r1))
		},
	},
	// float literal
	{
		regex(floatPat),
		func(s *stack, p string) {
			v, err := strconv.ParseFloat(p, 64)
			if err != nil {
				fatalf("bad float number %q", p)
			}
			s.pushX(v)
		},
	},
	// rune literal
	{
		regex("@."),
		func(s *stack, p string) {
			for _, c := range p[1:] {
				s.pushX(big.NewInt(int64(c)))
				break
			}
		},
	},
}

var ops = map[string][]interface{}{
	// constants
	"pi":       {math.Pi},
	"e":        {math.E},
	"phi":      {math.Phi},
	"nan":      {math.NaN()},
	"infinity": {math.Inf(1)},

	// functions from math package.
	"abs":       {math.Abs},
	"acos":      {math.Acos},
	"acosh":     {math.Acosh},
	"asin":      {math.Asin},
	"asinh":     {math.Asinh},
	"atan":      {math.Atan},
	"atan2":     {math.Atan2},
	"atanh":     {math.Atanh},
	"cbrt":      {math.Cbrt},
	"ceil":      {math.Ceil},
	"copysign":  {math.Copysign},
	"cos":       {math.Cos},
	"cosh":      {math.Cosh},
	"dim":       {math.Dim},
	"erf":       {math.Erf},
	"erfc":      {math.Erfc},
	"exp":       {math.Exp},
	"exp2":      {math.Exp2},
	"expm1":     {math.Expm1},
	"floor":     {math.Floor},
	"gamma":     {math.Gamma},
	"hypot":     {math.Hypot},
	"ilogb":     {math.Ilogb},
	"j0":        {math.J0},
	"j1":        {math.J1},
	"log":       {math.Log},
	"log10":     {math.Log10},
	"log1p":     {math.Log1p},
	"log2":      {math.Log2},
	"logb":      {math.Logb},
	"max":       {math.Max},
	"min":       {math.Min},
	"mod":       {math.Mod},
	"nextafter": {math.Nextafter},
	"pow":       {math.Pow},
	"remainder": {math.Remainder},
	"signbit":   {math.Signbit},
	"sin":       {math.Sin},
	"sinh":      {math.Sinh},
	"sqrt":      {math.Sqrt},
	"tan":       {math.Tan},
	"tanh":      {math.Tanh},
	"trunc":     {math.Trunc},
	"y0":        {math.Y0},
	"y1":        {math.Y1},

	// float64 functions we add.
	"quo": {mathQuo},
	"mul": {mathMul},
	"rem": {math.Remainder},
	"neg": {mathNeg},
	"add": {mathAdd},
	"sub": {mathSub},

	// conversion functions
	"int":   {cvtInt},
	"float": {cvtFloat},
	"rat":   {cvtRat},
}

var alias = map[string]string{
	"_": "neg",
	"-": "sub",
	"+": "add",
	"/": "quo",
	"x": "mul",
	"%": "rem",
	"!": "factorial",
}

func main() {
	addMethods(reflect.ValueOf((*big.Int)(nil)).Type())
	addMethods(reflect.ValueOf((*big.Rat)(nil)).Type())

	s := &stack{format: "%v"}
	s.interp(os.Args[1:])
	for _, v := range s.items {
		fmt.Println(v)
	}
}

func addMethods(t reflect.Type) {
	for i := 0; i < t.NumMethod(); i++ {
		addMethod(t.Method(i))
	}
}

func addMethod(m reflect.Method) {
	if m.PkgPath != "" {
		return
	}
	ft := m.Type
	rcvr := ft.In(0)
	for j := 1; j < ft.NumIn(); j++ {
		if ft.In(j) != rcvr {
			return
		}
	}
	for j := 0; j < ft.NumOut(); j++ {
		if ft.Out(j) != rcvr {
			return
		}
	}
	name := strings.ToLower(m.Name)
	ops[name] = append(ops[name], m.Func.Interface())
}

var bigOne = big.NewInt(1)

type stack struct {
	format   string
	minDepth int
	items    []value
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

var errStackEmpty = errors.New("stack empty")

func (s *stack) popN(n int) []value {
	d := len(s.items) - n
	if d < 0 {
		panic(errStackEmpty)
	}
	v := s.items[d:]
	s.items = s.items[0:d]
	if d < s.minDepth {
		s.minDepth = d
	}
	return v
}

func (s *stack) interp(args []string) {
nextArg:
	for _, a := range args {
		// TODO if a == "[" ... repetition op
		// operators
		if vs := ops[a]; vs != nil {
			s.exec(a, vs)
			continue nextArg
		}
		// aliases
		if b := alias[a]; b != "" {
			s.exec(a, ops[b])
			continue nextArg
		}
		// patterns
		for _, p := range patterns {
			if p.pat.MatchString(a) {
				s.exec(a, []interface{}{p.f})
				continue nextArg
			}
		}
		fatalf("unknown argument %q", a)
	}
}

func (s *stack) exec(name string, vs []interface{}) {
	defer func() {
		e := recover()
		if e == nil {
			return
		}
		if e == errStackEmpty {
			fatalf("stack underflow on %q", name)
		}
		panic(e)
	}()
	// Check whether operator is a constant or is
	// generic, in which case we just call it.
	switch v := vs[0].(type) {
	case *big.Int, *big.Rat, float64:
		s.pushX(v)
		return
	case func(*stack, string):
		v(s, name)
		return
	}

	t0 := reflect.ValueOf(vs[0]).Type()
	numIn := t0.NumIn()
	if t0.In(0).Kind() == reflect.Ptr {
		// methods of *big.Int and *big.Rat have their first argument
		// as receiver.
		numIn--
	}
	arg := s.popN(numIn)
	f := vs[0]
	if len(vs) > 0 {
		argt := reflect.TypeOf(arg[0].v)
		// The first argument determines the operator to use if there's
		// more than one, but we default to the first operator if
		// none matches.
		for _, v := range vs[1:] {
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

type value struct {
	v      interface{}
	format string
}

func (v value) Format(f fmt.State, c rune) {
	if true || f.Flag('#') {
		fmt.Fprintf(f, "%s[" + v.format +"]", typeName(v.v), v.v)
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

func mathAdd(x, y float64) float64 {
	return x + y
}

func mathSub(x, y float64) float64 {
	return x - y
}

func mathMul(x, y float64) float64 {
	return x * y
}

func mathQuo(x, y float64) float64 {
	return x / y
}

func mathNeg(x float64) float64 {
	return -x
}

func cvtInt(s *stack, _ string) {
	v := s.pop()
	s.push(v, v.toInt())
}

func cvtFloat(s *stack, _ string) {
	v := s.pop()
	s.push(v, v.toFloat())
}

func cvtRat(s *stack, _ string) {
	v := s.pop()
	s.push(v, v.toRat())
}

func fatalf(format string, args ...interface{}) {
	m := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "fc: %s\n", m)
	os.Exit(2)
}

func regex(s string) *regexp.Regexp {
	return regexp.MustCompile("^(" + s + ")$")
}
