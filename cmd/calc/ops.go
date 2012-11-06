package main

import (
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var ops = map[string][]interface{}{
	// constants
	"pi":       {math.Pi},
	"e":        {math.E},
	"phi":      {math.Phi},
	"nan":      {math.NaN()},
	"infinity": {math.Inf(1)},

	// functions from math package.
	"abs":        {math.Abs, (*big.Int).Abs, (*big.Rat).Abs},
	"acosh":      {math.Acosh},
	"acos":       {math.Acos},
	"add":        {mathAdd, (*big.Int).Add, (*big.Rat).Add},
	"and":        {(*big.Int).And},
	"andnot":     {(*big.Int).AndNot},
	"asinh":      {math.Asinh},
	"asin":       {math.Asin},
	"atan2":      {math.Atan2},
	"atanh":      {math.Atanh},
	"atan":       {math.Atan},
	"cbrt":       {math.Cbrt},
	"ceil":       {math.Ceil},
	"copysign":   {math.Copysign},
	"cosh":       {math.Cosh},
	"cos":        {math.Cos},
	"dim":        {math.Dim},
	"div":        {(*big.Int).Div},
	"divmod":     {(*big.Int).DivMod},
	"erfc":       {math.Erfc},
	"erf":        {math.Erf},
	"exp2":       {math.Exp2},
	"expm1":      {math.Expm1},
	"exp":        {math.Exp},
	"expmod":     {(*big.Int).Exp}, // see also "pow", below
	"float":      {cvtFloat},
	"floor":      {math.Floor},
	"gamma":      {math.Gamma},
	"gcd":        {(*big.Int).GCD},
	"hypot":      {math.Hypot},
	"int":        {cvtInt},
	"inv":        {(*big.Rat).Inv},
	"j0":         {math.J0},
	"j1":         {math.J1},
	"log10":      {math.Log10},
	"log1p":      {math.Log1p},
	"log2":       {math.Log2},
	"logb":       {math.Logb},
	"log":        {math.Log},
	"lsh":        {bigLsh},
	"max":        {math.Max},
	"min":        {math.Min},
	"mod":        {math.Mod, (*big.Int).Mod},
	"modinverse": {(*big.Int).ModInverse},
	"mul":        {mathMul, (*big.Int).Mul, (*big.Rat).Mul},
	"neg":        {mathNeg, (*big.Int).Neg, (*big.Rat).Neg},
	"nextafter":  {math.Nextafter},
	"not":        {(*big.Int).Not},
	"or":         {(*big.Int).Or},
	"pow":        {math.Pow, intPow},
	"quo":        {mathQuo, (*big.Int).Quo, (*big.Rat).Quo},
	"quorem":     {(*big.Int).QuoRem},
	"rat":        {cvtRat},
	"remainder":  {math.Remainder},
	"rem":        {math.Remainder, (*big.Int).Rem},
	"rsh":        {bigRsh},
	"sinh":       {math.Sinh},
	"sin":        {math.Sin},
	"sqrt":       {math.Sqrt},
	"sub":        {mathSub, (*big.Int).Sub, (*big.Rat).Sub},
	"tanh":       {math.Tanh},
	"tan":        {math.Tan},
	"trunc":      {math.Trunc},
	"xor":        {(*big.Int).Xor},
	"y0":         {math.Y0},
	"y1":         {math.Y1},
}

func init() {
	ops["help"] = []interface{}{help} // break initialisation cycle
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

type pattern struct {
	pat *regexp.Regexp
	f   genericOp
}

const floatPat = `(([0-9]+(\.[0-9]+)?)|([0-9]*(\.[0-9]+)))([eE]-?[0-9]+)?`

var patterns = []pattern{
	// format specifier
	{
		regex("%.*[fgvedxXoObB]"),
		genericOp{1, 1, func(s *stack, p string) {
			// % has a special case - if the stack is empty, it sets
			// the default format output.
			if len(s.items) == 0 {
				s.format = p
			} else {
				v := s.pop()
				v.format = p
				s.push(v, v.v)
			}
		}},
	},
	// integer literal
	{
		regex("0[bB][01]+|0[xX][0-9a-fA-F]+|0[0-9]+|[0-9]+"),
		genericOp{0, 1, func(s *stack, p string) {
			i, ok := new(big.Int).SetString(p, 0)
			if !ok {
				s.fatalf("cannot convert %q to int", p)
			}
			s.pushX(i)
		}},
	},
	// rational literal
	{
		regex(floatPat + "/" + floatPat),
		genericOp{0, 1, func(s *stack, p string) {
			i := strings.Index(p, "/")
			r0, ok0 := new(big.Rat).SetString(p[0:i])
			r1, ok1 := new(big.Rat).SetString(p[i+1:])
			if !ok0 || !ok1 {
				s.fatalf("bad rational %q", p)
			}
			s.pushX(r0.Quo(r0, r1))
		}},
	},
	// float literal
	{
		regex(floatPat),
		genericOp{0, 1, func(s *stack, p string) {
			v, err := strconv.ParseFloat(p, 64)
			if err != nil {
				s.fatalf("bad float number %q", p)
			}
			s.pushX(v)
		}},
	},
	// rune literal
	{
		regex("@."),
		genericOp{0, 1, func(s *stack, p string) {
			for _, c := range p[1:] {
				s.pushX(big.NewInt(int64(c)))
				break
			}
		}},
	},
}

func bigLsh(z, x, y *big.Int) *big.Int {
	i := y.Int64()
	if i < 0 {
		panic("negative shift")
	}
	return z.Lsh(x, uint(i))
}

func bigRsh(z, x, y *big.Int) *big.Int {
	i := y.Int64()
	if i < 0 {
		panic("negative shift")
	}
	return z.Rsh(x, uint(i))
}

// We define intPow here because there's a naming
// clash between math (e ** x) and big (x ** y ^ m)
func intPow(z, x, y *big.Int) *big.Int {
	return z.Exp(x, y, nil)
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

var cvtInt = genericOp{1, 1, func(s *stack, _ string) {
	v := s.pop()
	s.push(v, v.toInt())
}}

var cvtFloat = genericOp{1, 1, func(s *stack, _ string) {
	v := s.pop()
	s.push(v, v.toFloat())
}}

var cvtRat = genericOp{1, 1, func(s *stack, _ string) {
	v := s.pop()
	s.push(v, v.toRat())
}}

func regex(s string) *regexp.Regexp {
	return regexp.MustCompile("^(" + s + ")$")
}
