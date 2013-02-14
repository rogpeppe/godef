package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

func main() {
	t0 := time.Now()
	b := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	wasPrefix := false
	fmt.Fprintf(out, "start %s\n", time.Now().Format(time.RFC1123Z))
	out.Flush()
	for{
		line, isPrefix, err := b.ReadLine()
		if err != nil {
			break
		}
		if !wasPrefix {
			d := time.Now().Sub(t0)
			msec := d / time.Millisecond
			sec := d / time.Second
			min := d / time.Minute
			fmt.Fprintf(out, "%d:%02d.%03d ", min, sec%60, msec%1000)
		}
		out.Write(line)
		if !isPrefix {
			out.WriteByte('\n')
			out.Flush()
		}
		wasPrefix = isPrefix
	}
	out.Flush()
}
