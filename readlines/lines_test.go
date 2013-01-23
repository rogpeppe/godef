package readlines_test

import (
	"bufio"
	"code.google.com/p/rog-go/readlines"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

var linesTests = []struct {
	input     string
	bufioSize int
	maxSize   int
	lines     []string
}{{
	input: `
one
two
three
`[1:],
	maxSize: 10,
	lines:   []string{"one", "two", "three"},
}, {
	input: `
01234567890123456789
01234567890123456
0123
`[1:],
	bufioSize: 16,
	maxSize:   18,
	lines:     []string{"012345678901234567", "01234567890123456", "0123"},
}, {
	input: `
0123456789abcdefghijklmnopqrstuvwxyz!@#$%%^&*
0123456789abcdefghijklmnopqrstuvwxyz!@#$%%^&*
`[1:],
	maxSize: 10,
	lines:   []string{"0123456789", "0123456789"},
}, {
	input:   "oneline",
	maxSize: 20,
	lines:   []string{"oneline"},
}, {
	input:   "\n\n",
	maxSize: 20,
	lines:   []string{"", ""},
}, {
	input:   `Peppé`,
	maxSize: 5,
	lines:   []string{"Pepp"},
},
}

func TestLines(t *testing.T) {
	for i, test := range linesTests {
		var r io.Reader = strings.NewReader(test.input)
		if test.bufioSize > 0 {
			r = bufio.NewReaderSize(r, test.bufioSize)
		}
		var lines []string
		err := readlines.Iter(r, test.maxSize, func(line []byte) error {
			lines = append(lines, string(line))
			return nil
		})
		if err != nil {
			t.Errorf("test %d; unexpected error: %v", i, err)
		}
		if !reflect.DeepEqual(lines, test.lines) {
			t.Errorf("test %d; want %q; got %q", i, test.lines, lines)
		}
	}
}

func ExampleIter() {
	input := `one two
three
four

five`
	r := strings.NewReader(input)
	readlines.Iter(r, 1024*1024, func(line []byte) error {
		fmt.Printf("%q\n", line)
		return nil
	})
	// Output:
	// "one two"
	// "three"
	// "four"
	// ""
	// "five"
}
