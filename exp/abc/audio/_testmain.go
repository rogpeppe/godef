package main

import "abc/audio"
import "testing"

var tests = []testing.Test {
	testing.Test{ "audio.TestParserWithPipes", audio.TestParserWithPipes },
	testing.Test{ "audio.TestConversion", audio.TestConversion },
}
var benchmarks = []testing.Benchmark {
}

func main() {
	testing.Main(tests);
	testing.RunBenchmarks(benchmarks)
}
