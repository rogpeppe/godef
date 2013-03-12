package main

import (
	"bufio"
	"flag"
	"log"
	"io"
	"strings"
	"fmt"
	"os"
	"time"
)

var (
	printMilliseconds = flag.Bool("ms", false, "print milliseconds instead of mm:ss.000")
	suppressFilenames = flag.Bool("n", false, "suppress printing of file names")
)

var usage = `usage: timestamp [flags] [file...]
If files are named, they are read and the timestamp output in
the named files is merged into one time sequence.

When used on a single file
`

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if args := flag.Args(); len(args) > 0 {
		if len(args) == 1 {
			*suppressFilenames = true
		}
		mergeFiles(flag.Args())
		return
	}
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
			printStamp(out, time.Now().Sub(t0))
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

func mergeFiles(files []string) {
	fs := make([]*bufio.Reader, len(files))
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			log.Fatalf("mergetime: cannot open file: %v", err)
		}
		fs[i] = bufio.NewReader(f)
	}
	out := readLines(fs[0], files[0])
	for i, f := range fs[1:] {
		out = merge(readLines(f, files[i+1]), out)
	}
	startLine := <-out
	if startLine.line != "start\n" {
		panic("no start")
	}
	stdout := bufio.NewWriter(os.Stdout)
	t0 := startLine.t
	fmt.Fprintf(stdout, "start %s\n", t0.Format(time.RFC1123Z))
	stdout.Flush()
	for line := range out {
		printStamp(stdout, line.t.Sub(t0))
		if !*suppressFilenames {
			fmt.Fprintf(stdout, "%s: ", line.name)
		}
		stdout.WriteString(line.line)
		stdout.Flush()
	}
}

func merge(c0, c1 <-chan line) <-chan line {
	out := make(chan line)
	go func() {
		defer close(out)
		var line0, line1 line
		r0, r1 := c0, c1
		ok0, ok1 := true, true
		for {
			if r0 != nil {
				line0, ok0 = <-r0
				r0 = nil
			}
			if r1 != nil {
				line1, ok1 = <-r1
				r1 = nil
			}
			switch {
			case !ok0 && !ok1:
				return
			case !ok0:
				out <- line1
				r1 = c1
			case !ok1:
				out <- line0
				r0 = c0
			default:
				if line0.t.Before(line1.t) {
					out <- line0
					r0 = c0
				} else {
					out <- line1
					r1 = 	c1
				}
			}
		}
	}()
	return out
}

type line struct {
	t time.Time
	line string
	name string
}

func readLines(r *bufio.Reader, name string) <-chan line {
	out := make(chan line)
	go func() {
		defer close(out)
		startLine, err := r.ReadString('\n')
		if err != nil || !strings.HasPrefix(startLine, "start ") {
			log.Printf("mergetime: cannot read start line")
			return
		}
		
		start, err := time.Parse(time.RFC1123Z, startLine[len("start "):len(startLine)-1])
		if err != nil {
			log.Printf("mergetime: cannot parse start line %q: %v", startLine, err)
		}
		out <- line{start, "start\n", name}
		prev := start
		for {
			s, err := r.ReadString('\n')
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("mergetime: read error: %v", err)
				break
			}
			i := strings.Index(s, " ")
			if i == -1 {
				log.Printf("mergetime: line has no timestamp: %q", s)
				out <- line{prev, s, name}
				continue
			}
			var d time.Duration
			if strings.Index(s[0:i], ":") >= 0 {
				var min, sec, millisec int
				if _, err := fmt.Sscanf(s[0:i], "%d:%d.%d", &min, &sec, &millisec); err != nil {
					log.Printf("mergetime: cannot parse timestamp on line %q", s)
					out <- line{prev, s, name}
					continue
				}
				d = time.Duration(min) * time.Minute +
					time.Duration(sec) * time.Second +
					time.Duration(millisec) * time.Millisecond
			} else {
				var millisec int64
				if _, err := fmt.Scanf(s[0:i], "%d", &millisec); err != nil {
					log.Printf("mergetime: cannot parse timestamp on line %q", s)
					out <- line{prev, s, name}
					continue
				}
				d = time.Duration(millisec) * time.Millisecond
			}
			t := start.Add(d)
			out <- line{t, s[i+1:], name}
			prev = t
		}
	}()
	return out
}

func printStamp(w io.Writer, d time.Duration) {
	if *printMilliseconds {
		fmt.Fprintf(w, "%010d ", d / 1e6)
		return
	}
	msec := d / time.Millisecond
	sec := d / time.Second
	min := d / time.Minute
	fmt.Fprintf(w, "%d:%02d.%03d ", min, sec%60, msec%1000)
}
