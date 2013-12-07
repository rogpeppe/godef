package main

import (
	"code.google.com/p/goplan9/plan9/acme"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"strings"
)

// We would use io.Copy except for a bug in acme
// where it crashes when reading trying to read more
// than the negotiated 9P message size.
func copyBody(w io.Writer, win *acme.Win) error {
	buf := make([]byte, 8000)
	for {
		n, err := win.Read("body", buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if _, err := w.Write(buf[0:n]); err != nil {
			return fmt.Errorf("write error: %v", err)
		}
	}
	return nil
}

func acmeCurrentWin() (*acme.Win, error) {
	winid := os.Getenv("winid")
	if winid == "" {
		return nil, fmt.Errorf("$winid not set - not running inside acme?")
	}
	id, err := strconv.Atoi(winid)
	if err != nil {
		return nil, fmt.Errorf("invalid $winid %q", winid)
	}
	if err := setNameSpace(); err != nil {
		return nil, err
	}
	win, err := acme.Open(id, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot open acme window: %v", err)
	}
	return win, nil
}

func runeOffset2ByteOffset(b []byte, off int) int {
	r := 0
	for i, _ := range string(b) {
		if r == off {
			return i
		}
		r++
	}
	return len(b)
}

func setNameSpace() error {
	if ns := os.Getenv("NAMESPACE"); ns != "" {
		return nil
	}
	ns, err := nsFromDisplay()
	if err != nil {
		return fmt.Errorf("cannot get name space: %v", err)
	}
	os.Setenv("NAMESPACE", ns)
	return nil
}

// taken from src/lib9/getns.c
// This should go into goplan9/plan9/client.
func nsFromDisplay() (string, error) {
	disp := os.Getenv("DISPLAY")
	if disp == "" {
		// original code had heuristic for OS X here;
		// we'll just assume that and fail anyway if it
		// doesn't work.
		disp = ":0.0"
	}
	// canonicalize: xxx:0.0 => xxx:0
	if i := strings.LastIndex(disp, ":"); i >= 0 {
		if strings.HasSuffix(disp, ".0") {
			disp = disp[:len(disp)-2]
		}
	}

	// turn /tmp/launch/:0 into _tmp_launch_:0 (OS X 10.5)
	disp = strings.Replace(disp, "/", "_", -1)

	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("cannot get current user name: %v", err)
	}
	ns := fmt.Sprintf("/tmp/ns.%s.%s", u.Username, disp)
	_, err = os.Stat(ns)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("no name space directory found")
	}
	if err != nil {
		return "", fmt.Errorf("cannot stat name space directory: %v", err)
	}
	// heuristics for checking permissions and owner of name space
	// directory omitted.
	return ns, nil
}
