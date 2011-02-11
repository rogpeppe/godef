%{

package abc
import (
	"io"
	"fmt"
	"scanner"
)

// command flags args
//
// pipe node node
//	a | b
//	-> {b -in $tmp; a -out $=>tmp}
//
// assign name node
//	a = x
//	-> x -out $>a
//	a := x
//	-> x -out $=>a

// autoconvert type1 type2 name
// bugger: need to register types...

//map[*Type] 

//find shortest path

/*
	examples:
	a := {(blah=out) foo -blah $>blah}
	renaming: better to have internal or external on left of assignment?
	i think: inner=outer
	
	{(blah=out) foo -x $>blah} -out $>x

	confusing difference:
		{
			blah=$out
			foo -x $>blah		# illegal
		}

		{
			(out=blah)
			foo -x $>blah
		}

	decl foo [+x string] [-z] [-o] 

	is it a problem conflating environment variable names and
	"environment" options?
	what syntax to use to specify an environment option, if any?
*/

type argtype int
type unittype int 
const (
	aEndpoint = argtype(iota)
	aBlock
	aString
	aQstring
	aQblock

	uSimple = unittype(iota)
	uAssign
	uBlock
)

type assign struct {
	name string
	input *pipe
}

type pipe struct {
	input *pipe
	unit *unit
}

type unit struct {
	typ unittype
	varname string
	name string
	args *arg
}

type arg struct {
	typ argtype
	argname string
	local bool
	value string
	gender Gender
	block *pipe
	qblock *units
	next *arg
}

type units struct {
	cmd *assign
	next *units
}

type abcLex struct {
	scanner.Scanner
	lasttok int
	result *assign
}

%}

%union {
	block *units
	cmd *unit
	pipe *pipe
	arg *arg
	word string
	assign *assign
}

%token <word> tWORD tQWORD tFLAG tEND tDECL
%token tDECL tERROR
%type <assign> assign line
%type <pipe> pipe
%type <cmd> cmd simple
%type <arg> args arg literal var block1
%type <block> lines
%%
unit: line ';' {
		yylex.(*abcLex).result = $line;
		return 0
	}
	| line tEND {
		yylex.(*abcLex).result = $line;
		return 0
	}

line: pipe {
		if $pipe == nil {
			$$ = nil
		}else{
			$$ = &assign{"", $pipe}
		}
	}
	| assign
//	| error end

pipe:	{	// empty
		$$ = nil
	}
	| cmd {
		$$ = &pipe{nil, $cmd}
	}
	| pipe '|' cmd {
		$$ = &pipe{$pipe, $cmd}
	}

cmd: simple

assign: tWORD '=' pipe {
		$$ = &assign{$tWORD, $pipe}
	}

simple: tWORD args {
		$$ = &unit{typ: uSimple, name: $tWORD, args: revargs($args)}
	}
	| tDECL type {
		yylex.Error("no declarations yet")
		$$ = nil
	}
//	| block {
//		$$ = &unit{typ: uBlock, args: $block}
//	}

args:		{	// empty
		$$ = nil
	}
	| args arg {
		($arg).next = $args;
		$$ = $arg
	}

arg: literal
	| var
	| block1
	| tFLAG arg {
		if ($arg).argname != "" {
			yylex.Error("nested flags")
			$$ = nil
		}else{
			($arg).argname = $tFLAG;
			$$ = $arg
		}
	}
	| tDECL {
		$$ = &arg{typ: aString, value: $tDECL}
	}
	| '@' '{' environ lines '}' {
		// TODO: environ
		$$ = &arg{typ: aQblock, qblock: revlines($lines)}
	}

literal: tWORD {
		$$ = &arg{typ: aString, value: $tWORD}
	}
	| tQWORD {
		$$ = &arg{typ: aQstring, value: $tQWORD}
	}

var: '$' tWORD {
		$$ = &arg{typ: aEndpoint, value: $tWORD, gender: Female}
	}
	| '$' '>' tWORD {
		$$ = &arg{typ: aEndpoint, value: $tWORD, gender: Male}
	}
	| '$' '=' '>' tWORD{
		$$ = &arg{typ: aEndpoint, value: $tWORD, gender: Male, local: true}
	}

block1: '{' environ pipe '}'  {
		// TODO: environ
		$$ = &arg{typ: aBlock, block: $pipe}
	}

lines: line {
		$$ = &units{$line, nil}
	}
	| lines ';'  line {
		$$ = &units{$line, $lines}
	}

environ: 	// empty
	| '(' renames ')'

renames: rename
	| renames rename

rename: tWORD '=' tWORD

type: tWORD argtypes

argtypes:
	| argtypes argtype
	| argtypes '[' tFLAG argtype ']'

argtype: tWORD
	| '<' tWORD
	| '>' tWORD
	| blocktype

blocktype: '{' blocktypeargs types '}'

words:
	| words tWORD

blocktypeargs:
	| '(' words ')'

types:
	| types argtype

%%

func (l *abcLex) Lex(lval *yySymType) (tok int) {
again:
	switch t := l.Scan(); t {
	case scanner.Ident, scanner.Float, scanner.Int:
		w := l.TokenText()
		switch w {
		case "decl":
			tok = tDECL
		default:
			tok = tWORD
		}
		lval.word = w

	case scanner.String, scanner.RawString:
		lval.word = l.TokenText()
		tok = tQWORD

	case '-':
		oldws := l.Whitespace
		l.Whitespace = 0
		switch t := l.Scan(); t {
		case scanner.Ident:
			lval.word = l.TokenText()
			tok = tFLAG
		default:
			tok = tERROR
		}
		l.Whitespace = oldws

	case '\n':
		switch l.lasttok {		// {
		case tWORD, tQWORD, '}':
			tok = ';'
		default:
			goto again
		}
		
	case '<', '>', '{', '}', '@', '$', '=', ';', '|':
		tok = t

	case scanner.EOF:
		tok = tEND

	default:
		tok = tERROR
	}
	l.lasttok = tok
	return
}

func (l *abcLex) Error(s string) {
	fmt.Printf("error at %s: %s\n", l, s)
}

func (l *abcLex) Init(r io.Reader) *abcLex {
	l.Scanner.Init(r)
	l.Whitespace &^= 1<<'\n'
	return l
}

func (l *abcLex) Parse() *assign {
	l.result = nil
	yyParse(l)
	return l.result
}

func revargs(a *arg) (r *arg) {
	for a != nil {
		next := a.next
		a.next = r
		r = a
		a = next
	}
	return
}

func revlines(a *units) (r *units) {
	for a != nil {
		next := a.next
		a.next = r
		r = a
		a = next
	}
	return
}

// command flags args
//
// pipe node node
//	a | b
//	-> {b -in $tmp; a -out $=>tmp}
//
// assign name node
//	a = x
//	-> x -out $>a
//	a := x
//	-> x -out $=>a
