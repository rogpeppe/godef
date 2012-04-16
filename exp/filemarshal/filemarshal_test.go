package filemarshal

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"testing"
)

var tests = map[string]func(*T, Encoder) func(*T, Decoder){
	"oneFile":            oneFile,
	"mismatchedReceiver": mismatchedReceiver,
	"largeFile":          largeFile,
	"noFiles":            noFiles,
}

// TODO: test duplicate files

const testData = "hello, world"
const testString = "test"

func oneFile(t *T, enc Encoder) func(t *T, dec Decoder) {
	type One struct {
		S string
		F *File
	}
	testEncode(t, enc, One{testString, newFile(t, []byte(testData))})
	return func(t *T, dec Decoder) {
		var x *One
		testDecode(t, dec, &x)

		if x == nil || x.S != testString {
			t.Fatal("decode gave invalid data")
		}

		testContents(t, x.F, []byte(testData))
	}
}

func noFiles(t *T, enc Encoder) func(t *T, dec Decoder) {
	testEncode(t, enc, 1234)
	return func(t *T, dec Decoder) {
		var i int
		testDecode(t, dec, &i)
		if i != 1234 {
			t.Fatal("expected 1234; got ", i)
		}
	}
}

func mismatchedReceiver(t *T, enc Encoder) func(t *T, dec Decoder) {
	type X1 struct {
		F3 *File
		F1 *File
		F2 *File
	}
	d1 := []byte(testData + "1")
	d2 := []byte(testData + "2")
	testEncode(t, enc, X1{
		F1: newFile(t, d1),
		F2: newFile(t, d2),
		F3: newFile(t, []byte("discard"))})
	return func(t *T, dec Decoder) {
		type X2 struct {
			F2 *File
			F1 *File
			G  *File
		}
		var x X2
		testDecode(t, dec, &x)
		testContents(t, x.F1, d1)
		testContents(t, x.F2, d2)
	}
}

func largeFile(t *T, enc Encoder) func(t *T, dec Decoder) {
	data := make([]byte, 20000)
	for i := range data {
		data[i] = byte(i)
	}
	f := newFile(t, data)
	testEncode(t, enc, f)
	return func(t *T, dec Decoder) {
		var x *File
		testDecode(t, dec, &x)
		testContents(t, x, data)
	}
}

func TestEncodeDecode(tt *testing.T) {
	t := &T{t: tt}
	var buf bytes.Buffer

	t1 := t.Push("gob")
	testEncodeDecode(t1, gob.NewEncoder(&buf), gob.NewDecoder(&buf))

	buf.Reset()
	t1 = t.Push("json")
	testEncodeDecode(t1, json.NewEncoder(&buf), json.NewDecoder(&buf))
	runtime.GC()
}

func testEncodeDecode(t *T, enc Encoder, dec Decoder) {
	enc = NewEncoder(enc)
	decTests := make(map[string]func(*T, Decoder), len(tests))
	for name, test := range tests {
		t := t.Push(name)
		decTests[name] = test(t, enc)
	}

	dec = NewDecoder(dec)

	for name, test := range decTests {
		t := t.Push(name)
		test(t, dec)
	}
}

func testContents(t *T, f *File, data []byte) {
	if f == nil {
		t.Fatal("got nil file")
	}
	osf := f.File()
	osf.Seek(0, 0)
	actualData, err := ioutil.ReadAll(osf)
	if err != nil {
		t.Fatalf("read %q (actual %q) failed: %v", f.Name, osf.Name(), err)
	}
	if string(actualData) != string(data) {
		t.Fatalf("wrong data; expected %q; got %q", data, actualData)
	}
}

func newFile(t *T, data []byte) *File {
	tf, err := ioutil.TempFile("", "fmtest")
	if err != nil {
		t.Fatal("cannot create temporary file:", err)
	}
	f := NewFile(tf)
	_, err = tf.Write(data)
	if err != nil {
		t.Fatal("write failed:", err)
	}
	runtime.SetFinalizer(f, func(f *File) {
		os.Remove(f.File().Name())
	})
	return f
}

func testEncode(t *T, enc Encoder, x interface{}) {
	if err := enc.Encode(x); err != nil {
		t.Fatal("encode failed:", err)
	}
}

func testDecode(t *T, dec Decoder, x interface{}) {
	if err := dec.Decode(x); err != nil {
		t.Fatal("decode failed:", err)
	}
}

type T struct {
	t      *testing.T
	prefix string
	next   *T
}

func (t *T) Fatal(args ...interface{}) {
	msg := fmt.Sprint(args...)
	if t.prefix != "" {
		msg = t.prefix + ": " + msg
	}
	t.t.Fatal(msg)
}

func (t *T) Fatalf(f string, args ...interface{}) {
	t.Fatal(fmt.Sprintf(f, args...))
}

func (t *T) Push(name string) *T {
	p := t.prefix
	if p != "" {
		p += ": "
	}
	return &T{t.t, p + name, t}
}
