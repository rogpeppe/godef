package loopback

import (
	"reflect"
	"strings"
	"testing"
)

var noopts Options

func TestParseNoOptions(t *testing.T) {
	in, out, actual, err := parseNetwork("tcp")
	if err != nil {
		t.Fatal("parseNetwork error: ", err)
	}
	if !reflect.DeepEqual(in, noopts) || !reflect.DeepEqual(out, noopts) {
		t.Fatalf("some options were set; got in: %v, out: %v", in, out)
	}
	if actual != "tcp" {
		t.Fatal("actual net name; expect %q got %q", "tcp", actual)
	}
}

var allopts = Options{
	ByteDelay: 1,
	Latency:   2,
	MTU:       3,
	InLimit:   4,
	OutLimit:  5,
}

var optStrings = []string{
	"bytedelay=1",
	"latency=2",
	"mtu=3",
	"inlimit=4",
	"outlimit=5",
}

func TestParseSomeOptions(t *testing.T) {
	testWithPrefix(t, optStrings, allopts, allopts, "tcp")
	testWithPrefix(t, mapStrings(optStrings, func(s string) string { return "in." + s }), allopts, noopts, "tcp")
	testWithPrefix(t, mapStrings(optStrings, func(s string) string { return "out." + s }), noopts, allopts, "tcp")
}

func mapStrings(a []string, f func(string) string) []string {
	b := make([]string, len(a))
	for i, s := range a {
		b[i] = f(s)
	}
	return b
}

func testWithPrefix(t *testing.T, attrs []string, inOpts, outOpts Options, actualNet string) {
	net := "[" + strings.Join(attrs, ",") + "]" + actualNet
	in, out, actual, err := parseNetwork(net)
	if err != nil {
		t.Fatalf("parseNetwork error parsing %q: %v", net, err)
	}
	if !reflect.DeepEqual(in, inOpts) {
		t.Errorf("in options for %q; expected %v got %v", net, allopts, in)
	}
	if !reflect.DeepEqual(out, outOpts) {
		t.Errorf("out options for %q; expected %v got %v", net, allopts, out)
	}
	if actual != actualNet {
		t.Errorf("actualnet for %q, expected %q got %q", net, actualNet, actual)
	}
}
