package main

import (
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/token"
	. "launchpad.net/gocheck"
	"testing"
)

type suite struct{}

var _ = Suite(suite{})

func TestAll(t *testing.T) {
	TestingT(t)
}

var parseSymLineTests = []struct {
	in     string
	expect symLine
	err    string
}{{
	in: "foo.go:23:45:\t x/bar.go:99:100 \tfoo/bar \tarble/bletch \tX.Y\tlocalvar+\t func(int) bool",
	expect: symLine{
		long: true,
		pos: token.Position{
			Filename: "foo.go",
			Line:     23,
			Column:   45,
		},
		referPos: token.Position{
			Filename: "x/bar.go",
			Line:     99,
			Column:   100,
		},
		exprPkg:  "foo/bar",
		referPkg: "arble/bletch",
		local:    true,
		kind:     ast.Var,
		plus:     true,
		expr:     "X.Y",
		exprType: "func(int) bool",
	},
}, {
	in: "x/y/z:1:0: program files/arble.go:3:45 x y z const",
	expect: symLine{
		long: true,
		pos: token.Position{
			Filename: "x/y/z",
			Line:     1,
			Column:   0,
		},
		referPos: token.Position{
			Filename: "program files/arble.go",
			Line:     3,
			Column:   45,
		},
		exprPkg:  "x",
		referPkg: "y",
		local:    false,
		kind:     ast.Con,
		plus:     false,
		expr:     "z",
		exprType: "",
	},
}, {
	in: "x.go:2:4: old new",
	expect: symLine{
		long: false,
		pos: token.Position{
			Filename: "x.go",
			Line:     2,
			Column:   4,
		},
		expr:    "old",
		newExpr: "new",
	},
}, {
	in:  "x/y/z:1:0: f.go:3:4 x y z xxx",
	err: `invalid kind "xxx"`,
}, {
	in:  "x/y/z:1:0: f.go:4.5 x y z xxx  ",
	err: "invalid line",
}}

func (suite) TestParseSymLine(c *C) {
	for i, test := range parseSymLineTests {
		c.Logf("test %d", i)
		sl, err := parseSymLine(test.in)
		if test.err != "" {
			c.Assert(err, ErrorMatches, test.err)
		} else {
			c.Assert(err, IsNil)
			c.Assert(sl, DeepEquals, &test.expect)
			s := sl.String()
			sl2, err := parseSymLine(s)
			c.Assert(err, IsNil)
			c.Assert(sl2, DeepEquals, sl)
		}
	}
}

// TODO
//func (suite) TestList(c *C) {
//	cwd, err := os.Getwd()
//	c.Assert(err, IsNil)
//	oldGoPath = os.Getenv("GOPATH")
//	
//}
