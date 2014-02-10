package main

func init() {
	addTestCases(errgoTests, errgo)
}

var errgoTests = []testCase{{
	Name: "errgo.0",
	In: `package main

import (
	"errors"
	"fmt"
	"github.com/errgo/errgo"
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
		return 0, err // A comment
	}
	return 24, nil
}
`,
	Out: `package main

import (
	"fmt"
	"launchpad.net/errgo/errors"
)

var errSomething = errors.New("foo")

func f() error {
	if err := foo(); err != nil {
		return errors.Wrapf(err, "failure")
	}
	errors.Newf("foo: %s, %s", arg1, arg2)
	errors.WrapMsg(err, "blah")
	errors.Wrapf(err, "blah: %s, %s", arg1, arg2)
	return errors.Newf("cannot something: %s, %s", x, y)
}

func wrapper() (int, error) {
	if x, err := foo(); err != nil {
		return 0, errors.Wrap(err)
	}
	if err := foo(); err != nil {
		return 0, errors. // A comment
					Wrap(err)
	}
	return 24, nil
}
`,
}}
