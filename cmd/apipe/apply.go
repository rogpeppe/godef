// The apipe command pipes the contents of the current acme window
// through its argument shell command and updates them to the result
// by applying minimal changes.
//
// For example:
// 
//	apipe gofmt
//
// will alter only the pieces of source code that
// have changed, leaving the rest untouched.
package main

import (
	"bufio"
	"bytes"
	"code.google.com/p/goplan9/plan9/acme"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"unicode/utf8"
)

func main() {
	if err := Main(); err != nil {
		fmt.Fprintf(os.Stderr, "apipe: %v", err)
		os.Exit(1)
	}
}

func Main() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: apipe cmd [arg...]")
	}
	cmdArgs := os.Args[1:]
	win, err := acmeCurrentWin()
	if err != nil {
		return err
	}
	defer win.CloseFiles()
	bodyFile, err := ioutil.TempFile("", "")
	if err != nil {
		return err
	}
	if err := copyBody(bodyFile, win); err != nil {
		return err
	}
	bodyFile.Seek(0, 0)
	pcmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	pcmd.Stdin = bodyFile
	pcmd.Stderr = os.Stderr

	cmd := exec.Command("diff", bodyFile.Name(), "-")
	cmd.Stdin, err = pcmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	diffOut, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start diff: %v", err)
	}
	if err := pcmd.Start(); err != nil {
		return fmt.Errorf("cannot start %q: %v", cmdArgs[0], err)
	}
	// TODO this doesn't appear to have the desired effect -
	// changes made after "nomark" don't seem to be undoable.
	//	if _, err := win.Write("ctl", []byte("nomark")); err != nil {
	//		return err
	//	}
	//	defer win.Write("ctl", []byte("mark"))
	err = apply(diffOut, func(addr string, data []byte) error {
		if _, err := win.Write("addr", []byte(addr)); err != nil {
			return fmt.Errorf("cannot set address %q: %v", addr, err)
		}
		if err := writeData(win, data); err != nil {
			return fmt.Errorf("cannot write data: %v", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func writeData(win *acme.Win, data []byte) error {
	if len(data) == 0 {
		_, err := win.Write("data", nil)
		return err
	}
	for len(data) > 0 {
		d := data
		if len(d) > 8000 {
			d = trimIncompleteRune(d[0:8000])
		}
		n, err := win.Write("data", d)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

// trimIncompleteRune returns b with any trailing
// incomplete rune sliced off.
func trimIncompleteRune(b []byte) []byte {
	i := len(b) - utf8.UTFMax
	if i < 0 {
		i = 0
	}
	lastStart := len(b)
	for ; i < len(b); i++ {
		if r, n := utf8.DecodeRune(b[i:]); r != utf8.RuneError || n > 1 {
			lastStart = len(b)
			continue
		}
		if utf8.RuneStart(b[i]) {
			lastStart = i
		}
	}
	return b[0:lastStart]
}

func quote(s string) string {
	s = strconv.Quote(s)
	return s[1 : len(s)-1]
}

var cmdPat = regexp.MustCompile(`^([0-9]+)(,([0-9]+))?([acd])([0-9]+)(,([0-9]+))?$`)

type diffOp struct {
	n1, n2, n3, n4 int
	op             rune
}

func apply(diffOut io.Reader, edit func(addr string, data []byte) error) error {
	scan := bufio.NewScanner(diffOut)
	var buf []byte
	offset := 0
	for scan.Scan() {
		op, err := parseDiffOp(scan.Text())
		if err != nil {
			return err
		}
		if op.op == 'c' || op.op == 'd' {
			if err := eatLines(scan, "< ", op.n2-op.n1+1); err != nil {
				return err
			}
		}
		if op.op == 'c' {
			if err := eatLines(scan, "---", 1); err != nil {
				return err
			}
		}
		buf = buf[:0]
		if op.op != 'd' {
			for i := op.n3; i <= op.n4; i++ {
				if !scan.Scan() {
					return io.ErrUnexpectedEOF
				}
				line := scan.Bytes()
				if !bytes.HasPrefix(line, []byte("> ")) {
					return fmt.Errorf("expected line starting with '> ', got %q", line)
				}
				buf = append(buf, line[2:]...)
				buf = append(buf, '\n')
			}
		}
		var addr string
		if op.n1 == op.n2 {
			addr = fmt.Sprintf("%d", op.n1+offset)
			if op.op == 'a' {
				addr += "+#0"
			}
		} else {
			addr = fmt.Sprintf("%d,%d", op.n1+offset, op.n2+offset)
			if op.op == 'a' {
				return fmt.Errorf("append with multiple line source")
			}
		}
		if err := edit(addr, buf); err != nil {
			return err
		}
		switch op.op {
		case 'a':
			offset += op.n4 - op.n3 + 1
		case 'd':
			offset -= op.n2 - op.n1 + 1
		case 'c':
			offset += (op.n4 - op.n3 + 1) - (op.n2 - op.n1 + 1)
		}
	}
	return nil
}

func eatLines(scan *bufio.Scanner, prefix string, n int) error {
	bprefix := []byte(prefix)
	for i := 0; i < n; i++ {
		if !scan.Scan() {
			return io.ErrUnexpectedEOF
		}
		if !bytes.HasPrefix(scan.Bytes(), bprefix) {
			return fmt.Errorf("line %q does not have expected prefix %q", scan.Bytes(), bprefix)
		}
	}
	return nil
}

func parseDiffOp(cmd string) (diffOp, error) {
	var op diffOp
	r := cmdPat.FindStringSubmatch(cmd)
	if len(r) == 0 {
		return op, fmt.Errorf("%q is not a valid diff operation", cmd)
	}
	op.n1 = atoi(r[1])
	if r[3] != "" {
		op.n2 = atoi(r[3])
	} else {
		op.n2 = op.n1
	}
	op.op = rune(r[4][0])
	op.n3 = atoi(r[5])
	if r[7] != "" {
		op.n4 = atoi(r[7])
	} else {
		op.n4 = op.n3
	}
	return op, nil
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(fmt.Errorf("unexpected bad number %q", s))
	}
	return n
}
