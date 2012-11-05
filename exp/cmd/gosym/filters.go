package main
import (
	"bufio"
	"os"
	"io"
	"fmt"
)

func readLines(f func(sl *symLine) error) error {
	r := bufio.NewReader(os.Stdin)
	for {
		line, isPrefix, err := r.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read error: %v", err)
		}
		if isPrefix {
			return fmt.Errorf("line too long")
		}
		sl, err := parseSymLine(string(line))
		if err != nil {
			return fmt.Errorf("cannot parse line %q: %v", line, err)
		}
		if err := f(sl); err != nil {
			return err
		}
	}
	return nil
}

type shortCmd struct {}

func init() {
	register("short", &shortCmd{}, nil)
}

func (c *shortCmd) run(ctxt *context, args []string) error {
	readLines(func(sl *symLine) error {
		if sl.long {
			sl.newExpr = sl.symName()
		}
		sl.long = false
		ctxt.printf("%s\n", sl)
		return nil
	})
	return nil
}
