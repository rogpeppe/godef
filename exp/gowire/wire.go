package wire

import (
	"os"
	"fmt"
	"reflect"
	"strings"
	"rog-go.googlecode.com/hg/values"
)


type Op struct {
	op     reflect.Value // function to call.
	ret  wireType		// ret.actualType==nil if no return type.
	opt   reflect.Type		// nil if no options.
	arg	[]wireType		// other arguments
	fields map[string]*option
}

type wireType struct {
	actualType reflect.Type
	elemType reflect.Type
}

type option struct {
	name     string
	fieldNum int
	typ wireType
}

type Param struct {
	Name  string
	Value interface{}
}

type Params []Param

type ConnKind byte

const (
	Const = iota
	Var
)

var osErrorType = reflect.TypeOf((*os.Error)(nil)).Elem()
var valueType = reflect.TypeOf((*values.Value)(nil)).Elem()

// NewOp creates a new Op from a function.
// The function must be of the form func(*R, T) os.Error
// where R and T are struct type. Each named field in
// T defines an parameter to the operation.
// Each named field in R defines a result value of the operation.
// The type of a parameter or result is that of the field's
// type unless the type is values.Value, which defines a
// a time-varying value - the actual type held in the Value
// must be defined in the following field, which must
// be named with the blank identifier, _.
//
// Unexported fields are ignored.
func NewOp(opFunc interface{}) *Op {
	fv := reflect.ValueOf(opFunc)
	ft := fv.Type()

	if ft.Kind() != reflect.Func {
		panicf("NewOp: expected function type, got %v", ft)
	}
	op := new(Op)
	op.op = fv
	nout := ft.NumOut()
	switch {
	case nout > 2:
		panicf("NewOp: too many return values")
	case nout > 0 && ft.Out(nout-1) != osErrorType:
		panicf("NewOp: last return value is not os.Error")
	case ft.IsVariadic():
		panicf("NewOp: variadic functions not supported")
	case nout > 1:
		var err os.Error
		op.ret, err = wireTypeOfType(ft.Out(0))
		if err != nil {
			panicf("NewOp: return value: %v", err)
		}
	}

	nin := ft.NumIn()
	a := 0
	// If the first argument is a struct, it gives options.
	if nin > 0 && ft.In(0).Kind() == reflect.Struct && !isValueType(ft.In(0)) {
		op.opt = ft.In(0)
		op.fields = make(map[string]*option)
		if op.opt.NumField() > 63 {
			panicf("too many options in %v", op.opt)
		}
		for i := 0; i < op.opt.NumField(); i++ {
			s := op.opt.Field(i)
			t, err := wireTypeOfType(s.Type)
			if err != nil {
				panicf("field %s: %v", s.Name, err)
			}
			c := &option{
				name:	strings.ToLower(s.Name),
				fieldNum: i,
				typ: t,
			}
			op.fields[c.name] = c
		}
		a++
	}
	op.arg = make([]wireType, nin-a)
	for i := a; i < nin; i++ {
		t, err := wireTypeOfType(ft.In(i))
		if err != nil {
			panicf("arg %d: %v", i, err)
		}
		op.arg[i-a] = t
	}
	return op
}

func isValueType(t reflect.Type) bool {
	wt, err := wireTypeOfType(t)
	return err == nil && wt.elemType != wt.actualType
}

// Call makes a call to the receiving Op.
func (op *Op) Call(convert NewConverter, ret interface{}, opts []Param, args ...interface{}) (err os.Error) {
	var argCvt []func()os.Error
	var argv []reflect.Value
	firstArg := 0
	if op.opt != nil || len(opts) > 0 {
		argv = make([]reflect.Value, len(op.arg) + 1)
		argv[0], err = op.makeOpts(convert, &argCvt, opts)
		if err != nil {
			return err
		}
		firstArg = 1
	}else{
		argv = make([]reflect.Value, len(op.arg))
	}
	err = op.makeArgs(convert, &argCvt, argv[firstArg:], args)
	if err != nil {
		return err
	}
	retCvt, err := op.makeRet(convert, ret)
	if err != nil {
		return err
	}
	// perform all argument conversions
	for _, cvt := range argCvt {
		if err := cvt(); err != nil {
			return err
		}
	}
	rv := op.op.Call(argv)
	if len(rv) == 0 {
		return nil
	}
	err = asOsError(rv[len(rv)-1])
	if err != nil {
		return err
	}
	if len(rv) > 1 {
		err = retCvt(rv[0])
	}
	return
}

var nilv reflect.Value

func (op *Op) makeOpts(convert NewConverter, argCvt *[]func()os.Error, opts []Param) (optv reflect.Value, err os.Error) {
	if op.opt == nil {
		if len(opts) > 0 {
			err = fmt.Errorf("invalid parameter name %q", opts[0].Name)
		}
		return
	}
	optv = reflect.New(op.opt).Elem()
	set := int64(0)
	for _, p := range opts {
		c, ok := op.fields[p.Name]
		if !ok {
			return nilv, fmt.Errorf("invalid parameter name %q", p.Name)
		}
		m := int64(1) << uint(c.fieldNum)
		if set&m != 0 {
			return nilv, fmt.Errorf("duplicate parameter %q", p.Name)
		}
		set |= m
		fv := optv.Field(c.fieldNum)
		pv := reflect.ValueOf(p.Value)
		pt, err := wireTypeOfType(pv.Type())
		if err != nil {
			return nilv, fmt.Errorf("parameter %q: %v", p.Name, err)
		}
		if pt.eq(c.typ) {
			fv.Set(pv)
			continue
		}
		// types are not equal, see if we can convert
		cvt, err := genConvertFunc(convert, c.typ, pt)
		if err != nil {
			return nilv, fmt.Errorf("param %q: cannot convert from %s to %s: %v", p.Name, pt, c.typ, err)
		}
		*argCvt = append(*argCvt, func() os.Error {
			return cvt(fv, pv)
		})
	}
	return optv, nil
}

func (op *Op) makeArgs(convert NewConverter, argCvt *[]func()os.Error, argv []reflect.Value, args []interface{}) os.Error {
	if len(args) != len(op.arg) {
		return fmt.Errorf("invalid parameter count; expected %d got %d", len(op.arg), len(args))
	}
	for i, argt := range op.arg {
		pv := reflect.ValueOf(args[i])
		pt, err := wireTypeOfType(pv.Type())
		if err != nil {
			return fmt.Errorf("arg %d: %v", err)
		}
		if pt.eq(argt) {
			argv[i] = pv
			continue
		}
		// types are not equal, see if we can convert
		cvt, err := genConvertFunc(convert, argt, pt)
		if err != nil {
			return fmt.Errorf("arg %d: %v", i, err)
		}
		av := reflect.New(argt.actualType).Elem()
		argv[i] = av
		*argCvt = append(*argCvt, func() os.Error { return cvt(av, pv) })
	}
	return nil
}

func (op *Op) makeRet(convert NewConverter, ret interface{}) (retCvt func(srcv reflect.Value) os.Error, err os.Error) {
	if !op.ret.isValid() {
		if ret != nil {
			err = fmt.Errorf("op returns nothing, but Call called expecting a value")
		}
		return
	}
	retv := reflect.ValueOf(ret)
	if retv.Kind() != reflect.Ptr {
		err = fmt.Errorf("return parameter expected pointer, got %v", retv.Type())
		return
	}
	retv = retv.Elem()
	rett, err := wireTypeOfType(retv.Type())
	if err != nil {
		err = fmt.Errorf("bad return type: %v", err)
		return
	}
	if rett.eq(op.ret) {
		retCvt = func(srcv reflect.Value) os.Error {
			retv.Set(srcv)
			return nil
		}
		return
	}
	cvt, err := genConvertFunc(convert, rett, op.ret)
	if cvt == nil {
		err = fmt.Errorf("cannot convert return type: %v", err)
		return
	}
	retCvt = func(srcv reflect.Value) os.Error {
		return cvt(retv, srcv)
	}
	return
}

func identity(dst, src reflect.Value) os.Error {
	dst.Set(src)
	return nil
}

// genConvertFunc returns a converter that can convert from
// srct to dstt, including between values.Value types.
func genConvertFunc(cvt NewConverter, dstt, srct wireType) (Converter, os.Error) {
	if dstt.eq(srct) {
		return identity, nil
	}
	src2dst := identity
	dst2src := identity
	if srct.elemType != dstt.elemType {
		src2dst = cvt.NewConverter(dstt.elemType, srct.elemType)
		if src2dst == nil {
			return nil, fmt.Errorf("no conversion found")
		}
		dst2src = cvt.NewConverter(srct.elemType, dstt.elemType)
	}
	switch {
	case dstt.isVar() && srct.isVar():
		// Var to Var (with conversion, otherwise dstt would equal srct)
		if dst2src == nil {
			return nil, os.ErrorString("no bidirectional conversion available")
		}
		lens := values.NewReflectiveLens(
			func(v reflect.Value) (reflect.Value, os.Error) {
				dstv := reflect.New(dstt.elemType).Elem()
				err := src2dst(dstv, v)
				return dstv, err
			},
			func(v reflect.Value) (reflect.Value, os.Error) {
				srcv := reflect.New(srct.elemType).Elem()
				err := dst2src(srcv, v)
				return srcv, err
			},
			srct.elemType,
			dstt.elemType,
		)
		return func(dstv, srcv reflect.Value) os.Error {
fmt.Printf("transforming %v(%v)->%v(%v)\n", srcv.Type(), srcv.Kind(), dstv.Type(), dstv.Kind())
			srcval := srcv.Field(0).Interface().(values.Value)
			transformedVal := values.Transform(srcval, lens)
			dstval := reflect.New(dstt.actualType).Elem()
			dstval.Field(0).Set(reflect.ValueOf(transformedVal))
			dstv.Set(dstval)
			return nil
		}, nil
	case srct.isVar():
		// Var to Const
		return nil, fmt.Errorf("cannot convert from variable (%v) to const (%v)", srct, dstt)
	case dstt.isVar() && srct.elemType == dstt.elemType:
		// Const to Var with no conversion
		return func(dstv, srcv reflect.Value) os.Error {
			val := values.NewConst(srcv.Interface(), dstt.elemType)
			dstv.Field(0).Set(reflect.ValueOf(val))
			return nil
		}, nil
	case dstt.isVar():
		// Const to Var with conversion
		return func(dstv, srcv reflect.Value) os.Error {
			v := reflect.New(dstt.elemType).Elem()
			if err := src2dst(v, srcv); err != nil {
				return err
			}
			val := values.NewConst(v.Interface(), dstt.elemType)
			dstv.Field(0).Set(reflect.ValueOf(val))
			return nil
		}, nil
	}
	// Const to Const
	return func(dstv, srcv reflect.Value) os.Error {
		return src2dst(dstv, srcv)
	}, nil
}

// asOsError returns the given value, which is known
// to be of type os.Error, as an os.Error interface.
func asOsError(v reflect.Value) os.Error {
	val, _ := v.Interface().(os.Error)
	return val
}

// isVar returns true if the t represents a time-varying type.
func (t wireType) isVar() bool {
	return t.actualType != t.elemType
}

func (t wireType) eq(t1 wireType) bool {
	return t.actualType == t1.actualType &&
		t.elemType == t1.elemType
}

func (t wireType) isValid() bool {
	return t.actualType != nil && t.elemType != nil
}

func (t wireType) String() string {
	return t.actualType.String()
}

// wireTypeOfType returns the "wire" type of x - if x is of the
// form struct {values.Value; _ T}, then the type
// is taken to represent a values.Value with element type T.
func wireTypeOfType(x reflect.Type) (wireType, os.Error) {
	if x.Kind() == reflect.Interface {
		return wireType{}, fmt.Errorf("wire type %v cannot be interface", x)
	}
	if x.Kind() != reflect.Struct {
		return wireType{x, x}, nil
	}
	if x.NumField() != 2 ||
		!x.Field(0).Anonymous ||
		x.Field(0).Type != valueType ||
		x.Field(1).Name != "_" {
		return wireType{x, x}, nil
	}
	return wireType{x, x.Field(1).Type}, nil
}

func panicf(f string, a ...interface{}) {
	panic(fmt.Errorf(f, a...))
}

