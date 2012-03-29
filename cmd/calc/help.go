package main

import (
	"fmt"
	"sort"
)

var help = genericOp{0, 0, func(*stack, string) {
	var lines []string
	for name, vs := range ops {
		lines = append(lines, fmt.Sprintf("%s[%d]", name, argCount(vs[0])))
	}
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Printf("%s\n", l)
	}
}}

func printAll() {
	for name, vs := range ops {
		fmt.Printf("%s\n", name)
		for _, v := range vs {
			fmt.Printf("\t%T\n", v)
		}
	}
}
