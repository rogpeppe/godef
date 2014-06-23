package main
import (
	"fmt"
	"strings"
)

func init() {
	addTestCases(errgoMaskTests, errgoMask)
	addTestCases(errgoCauseTests, errgoCause)
}

var errgoMaskTests = setPkgName([]testCase{{
	Name: "errgo-mask.0",
	In: `package main

import (
	"errors"
	"fmt"
	"github.com/errgo/errgo"
	gc "launchpad.net/gocheck"
)

var errSomething = errors.New("foo")

func f() error {
	if err := foo(); err != nil {
		return fmt.Errorf("failure: %v", err)
	}
	errgo.New("foo: %s, %s", arg1, arg2)
	errgo.Annotate(err, "blah")
	errgo.Annotatef(err, "blah: %s, %s", arg1, arg2)
	return fmt.Errorf("cannot something: %s, %s", x, y)
}

func wrapper() (int, error) {
	if x, err := foo(); err != nil {
		return 0, err
	}
	if err := foo(); err != nil {
		return 0, err
	}
	// A comment
	return 24, nil
}
`,
	Out: `package main

import (
	"fmt"
	$ERRGO
	gc "launchpad.net/gocheck"
)

var errSomething = errgo.New("foo")

func f() error {
	if err := foo(); err != nil {
		return errgo.Notef(err, "failure")
	}
	errgo.Newf("foo: %s, %s", arg1, arg2)
	errgo.NoteMask(err, "blah")
	errgo.Notef(err, "blah: %s, %s", arg1, arg2)
	return errgo.Newf("cannot something: %s, %s", x, y)
}

func wrapper() (int, error) {
	if x, err := foo(); err != nil {
		return 0, errgo.Mask(err)
	}
	if err := foo(); err != nil {
		return 0, errgo.Mask(

			// A comment
			err)
	}

	return 24, nil
}
`,
}, {
	Name: "errgo-mask - errgo.New",
	In: `package main

import errgo $ERRGO

var someErr = errgo.New("foo")
`,
	Out: `package main

import errgo $ERRGO

var someErr = errgo.New("foo")
`,
}})

var errgoCauseTests = setPkgName([]testCase{{
	Name: "errgo-cause.0",
	In: `package main

import (
	"errors"
	"fmt"
	gc "launchpad.net/gocheck"
)

func (*suite) TestSomething(c *gc.C) {
	err := foo()
	c.Check(err, gc.Equals, errSomething)
	c.Check(err, gc.Not(gc.Equals), errSomething)
	c.Check(err, gc.Equals, nil)
	c.Check(err, gc.Not(gc.Equals), nil)
}

func tester() error {
	if err := foo(); err == errSomething {
		return nil
	}
	if err := foo(); err == nil {
		return nil
	}
	if _, ok := err.(*foo); ok {
		return nil
	}
	return nil
}
`,
	Out: `package main

import (
	"fmt"
	$ERRGO
	gc "launchpad.net/gocheck"
)

func (*suite) TestSomething(c *gc.C) {
	err := foo()
	c.Check(errgo.Cause(err), gc.Equals, errSomething)
	c.Check(errgo.Cause(err), gc.Not(gc.Equals), errSomething)
	c.Check(err, gc.Equals, nil)
	c.Check(err, gc.Not(gc.Equals), nil)
}

func tester() error {
	if err := foo(); errgo.Cause(err) == errSomething {
		return nil
	}
	if err := foo(); err == nil {
		return nil
	}
	if _, ok := errgo.Cause(err).(*foo); ok {
		return nil
	}
	return nil
}
`,
}, {
	Name: "errgo-mask - errors on its own doesn't get rewritten",
	In: `package main

import "errors"

var someErr = errors.New("something")
`,
	Out: `package main

import "errors"

var someErr = errors.New("something")
`,
}})

func setPkgName(cases []testCase) []testCase {
	path := fmt.Sprintf("%q", errgoPkgPath)
	for i := range cases {
		c := &cases[i]
		c.In = strings.Replace(c.In, "$ERRGO", path, -1)
		c.Out = strings.Replace(c.Out, "$ERRGO", path, -1)
	}
	return cases
}
