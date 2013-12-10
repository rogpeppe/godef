package reverse_test

import (
	"bufio"

	"code.google.com/p/rog-go/reverse"

	"reflect"
	"strings"
	"testing"
)

var scanTests = []struct {
	text   string
	split  bufio.SplitFunc
	tokens []string
}{{
	text: `
hello one two three four five six seven
there

you one two three four five six seven` + "\r" + `
x

`,
	split: bufio.ScanLines,
	tokens: []string{
		"",
		"x",
		"you one two three four five six seven",
		"",
		"there",
		"hello one two three four five six seven",
		"",
	},
}, {
	text: `
hello one two three four five six seven
there

you one two three four five six seven` + "\r" + `
x

`,
	split: bufio.ScanWords,
	tokens: []string{
		"x",
		"seven",
		"six",
		"five",
		"four",
		"three",
		"two",
		"one",
		"you",
		"there",
		"seven",
		"six",
		"five",
		"four",
		"three",
		"two",
		"one",
		"hello",
	},
}}

func TestScan(t *testing.T) {
	for i, test := range scanTests {
		t.Logf("test %d", i)
		b := reverse.NewScanner(strings.NewReader(test.text))
		b.Split(test.split)
		var got []string
		for b.Scan() {
			got = append(got, b.Text())
		}
		if b.Err() != nil {
			t.Fatalf("error after scan: %v", b.Err())
		}
		if !reflect.DeepEqual(got, test.tokens) {
			t.Fatalf("token mismatch; got %q want %q", got, test.tokens)
		}
	}
}
