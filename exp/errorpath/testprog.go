// +build ignore

package main

import (
	"errors"
	"fmt"
	"os"

	"local/foo.bar"
)

func errorHandler(err *error) {}

// doScan does the real work for scanning without a format string.
func doScan(a []interface{}) (numProcessed int, err error) {
	defer errorHandler(&err)
	return
}

func main() {
	testProg()
}

func testProg() {
	switch len(os.Args) {
	case 0:
		something = func(string, ...interface{}) error {
			return errors.New("blah")
		}
	case 1:
		something = foo.GetErrorf()
	case 2:
		something = foo.MyErrorf
	case 3:
		something = GetErrorf()
	case 4:
		var b foo.B
		something = b.BError
	}
	Test(2)
}

type Foo struct {
	X int
}

var something = fmt.Errorf

func GetErrorf() func(string, ...interface{}) error {
	return func(string, ...interface{}) error {
		return nil
	}
}

type myError struct{}

var someError *myError

func (e *myError) Error() string { return "" }

func Test(x int) (*Foo, error) {
	y := x + 10
	var err error
	switch x {
	case 0:
		err = fmt.Errorf(os.Stderr.Name())
	case 1:
		return nil, something("err 1")
	case 2:
		return nil, fmt.Errorf("err 2")
	case 3:
		return nil, someError
	case 4:
		return &Foo{45 + y}, nil
	}
	if err != nil {
		return nil, err
	}
	return Other()
}

func Other() (*Foo, error) {
	return &Foo{99}, nil
}
