package wire

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
	"rog-go.googlecode.com/hg/values"
)

type Float64Value struct {
	values.Value
	_ float64
}

type IntValue struct {
	values.Value
	_ int
}

type StringValue struct {
	values.Value
	_ string
}

func newFloat64Value(x float64) (v Float64Value) {
	v.Value = values.NewValue(x, nil)
	return
}

func newIntValue(x int) (v IntValue) {
	v.Value = values.NewValue(x, nil)
	return
}

func newStringValue(x string) (v StringValue) {
	v.Value = values.NewValue(x, nil)
	return
}

//extra options
//options missing
//options of the wrong type
//extra args
//args missing
//invalid return type

//const to var
//const to var with transform
//var to var with transform

const (
	startValue = 100
	endValue = 110
)

func mustHaveError(t *testing.T, r interface{}, err os.Error) {
	if err == nil {
		t.Errorf("expected error got none")
	}
}

func mustNotHaveError(t *testing.T, r interface{}, err os.Error) {
	if err != nil {
		t.Errorf("expected success got error %v", err)
	}
}

type callTest struct {
	f string
	ret interface{}
	opts []Param
	args []interface{}
	check func(t *testing.T, r interface{}, err os.Error)
}


var callTests = []callTest{
	{"f", nil, nil, nil, mustNotHaveError},
	{"fr1", nil, nil, nil, mustNotHaveError},
	{"fav1", nil, nil, []interface{}{fmt.Sprint(startValue)}, mustNotHaveError},
	{"fav1",  nil, nil, []interface{}{100}, mustNotHaveError},
	{"fav1",  nil, nil, []interface{}{changingIntValue()}, mustNotHaveError},
}

func changingIntValue() IntValue {
	v := newIntValue(startValue)
	go func() {
		for i := startValue+1; i < endValue; i++ {
			time.Sleep(0.1e9)
			v.Set(i)
		}
		time.Sleep(0.1e9)
		v.Close()
		
	}()
	return v
}	

type testError string

func fatalf(f string, args ...interface{}){
	panic(testError(fmt.Sprintf(f, args...)))
}

func funcs(t *testing.T, wg *sync.WaitGroup, errs chan<- string) map[string]interface{} {
	start := func(f func()){
		wg.Add(1)
		go func(){
			defer func(){
				switch e := recover().(type) {
				case testError:
					errs <- string(e)
				case os.Error, string:
					panic(e)
				}
				wg.Done()
			}()
			f()
		}()
	}
		
	return map[string]interface{}{
		"f": func() {
		},

		"fr1": func() os.Error {
			return nil
		},

		"fav1": func(i StringValue) os.Error {
			start(func() {
				g := i.Getter()
				expect := startValue
				for {
					v, ok := g.Get()
					if !ok {
						break
					}
					if v != fmt.Sprint(expect) {
						fatalf("fav1: expected %v got %v", expect, v)
					}
					expect++
				}
			})
			return nil
		},

		"fr2": func() (int, os.Error) {
			return startValue, nil
		},

		"fac1": func (i int) {
			if i != startValue {
				fatalf("expected %v got %v", startValue, i)
			}
		},
	}
}

func newOp(t *testing.T, test callTest, f interface{}) *Op {
	defer func(){
		if e, ok := recover().(os.Error); ok {
			t.Fatalf("func %s: %v", test.f, e)
		}
	}()
	return NewOp(f)
}

func float64ToInt(dst, src reflect.Value) os.Error {
	dst.Set(reflect.ValueOf(int(src.Interface().(float64) + 0.5)))
	return nil
}

func intToFloat64(dst, src reflect.Value) os.Error {
	dst.Set(reflect.ValueOf(float64(src.Interface().(int))))
	return nil
}

func float64ToString(dst, src reflect.Value) os.Error {
	dst.Set(reflect.ValueOf(fmt.Sprint(src.Interface())))
	return nil
}

func stringToFloat64(dst, src reflect.Value) os.Error {
	f, err := strconv.Atof64(src.Interface().(string))
	if err != nil {
		return err
	}
	dst.Set(reflect.ValueOf(f))
	return nil
}

func TestFuncs(t *testing.T) {
	errs := make(chan string)
	done := make(chan bool)
	go func(){
		for e := range errs {
			t.Error(e)
		}
		done <- true
	}()
	wg := new(sync.WaitGroup)
	f := funcs(t, wg, errs)
	g := newGraphConversions()
	g.AddConversion(reflect.TypeOf(0), reflect.TypeOf(0.0), float64ToInt, "float64->int")
	g.AddConversion(reflect.TypeOf(0.0), reflect.TypeOf(0), intToFloat64, "int->float64")
	g.AddConversion(reflect.TypeOf(""), reflect.TypeOf(0.0), float64ToString, "float64->string")
	g.AddConversion(reflect.TypeOf(0.0), reflect.TypeOf(""), stringToFloat64, "string->float64")
	for i, test := range callTests {
fmt.Printf("test #%d\n", i)
		op := newOp(t, test, f[test.f])
		err := op.Call(g, test.ret, test.opts, test.args...)
		test.check(t, test.ret, err)
	}
	wg.Wait()
	close(errs)
	<-done
}

func add(opt struct {
	A int
	B Float64Value
}) (<-chan float64, os.Error) {
	c := make(chan float64)
	go func() {
		g := opt.B.Getter()
		for {
			v, ok := g.Get()
			if !ok {
				break
			}
			c <- v.(float64)
		}
		close(c)
	}()
	return c, nil
}

type P []Param

func TestAdd(t *testing.T) {
	op := NewOp(add)
	v := newFloat64Value(1.5)
	var r <-chan float64

	err := op.Call(naiveConversions{}, &r, P{{"a", 100}, {"b", v}})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	sync := make(chan bool)
	go func() {
		for x := range r {
			fmt.Printf("got %v\n", x)
		}
		fmt.Printf("end\n")
		sync <- true
	}()
	for i := 0.0; i < 10.0; i++ {
		v.Set(i)
		time.Sleep(0.1e9)
	}
	v.Close()
	<-sync
}

func nullConversion(dst, src reflect.Value) os.Error {
	return os.ErrorString("no conversion")
}

func TestGraph(t *testing.T) {
	type T1 int
	type T2 int
	type T3 int
	type T4 int
	type T5 int
	type T6 int
	g := newGraphConversions()
	add := func(v0, v1 interface{}) {
		if err := g.AddConversion(reflect.TypeOf(v0), reflect.TypeOf(v1), nullConversion, fmt.Sprintf("%T->%T", v1, v0)); err != nil {
			t.Fatalf("cannot add conversion %v->%v: %v", reflect.TypeOf(v1), reflect.TypeOf(v0), err)
		}
	}
	add(T5(0), T4(0))
	add(T4(0), T3(0))
	add(T3(0), T2(0))
	add(T2(0), T1(0))
	add(T5(0), T6(0))
	add(T6(0), T3(0))
	cvt := g.NewConverter(reflect.TypeOf(T5(0)), reflect.TypeOf(T1(0)))
	if cvt == nil {
		t.Fatalf("conversion failed")
	}
}

