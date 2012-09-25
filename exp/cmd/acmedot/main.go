// acmedot prints the address of the selection in the current
// acme window as two numbers.
package main

import (
	"code.google.com/p/goplan9/plan9/acme"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

func main() {
	winid := os.Getenv("winid")
	if winid == "" {
		fatal("$winid not set - not running inside acme?")
	}
	id, err := strconv.Atoi(winid)
	if err != nil {
		fatal("invalid $winid %q", winid)
	}
	setNameSpace()
	win, err := acme.Open(id, nil)
	if err != nil {
		fatal("cannot open acme window: %v", err)
	}
	defer win.CloseFiles()
	_, _, err = win.ReadAddr() // make sure address file is already open.
	if err != nil {
		fatal("cannot read address: %v", err)
	}
	err = win.Ctl("addr=dot")
	if err != nil {
		fatal("cannot set addr=dot: %v", err)
	}
	q0, q1, err := win.ReadAddr()
	if err != nil {
		fatal("cannot read address: %v", err)
	}
	fmt.Println(q0, q1)
}

func fatal(f string, args ...interface{}) {
	msg := fmt.Sprintf(f, args...)
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	os.Exit(1)
}

func setNameSpace() {
	if ns := os.Getenv("NAMESPACE"); ns != "" {
		return
	}
	ns, err := nsFromDisplay()
	if err != nil {
		fatal("cannot get name space: %v", err)
	}
	os.Setenv("NAMESPACE", ns)
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
	ns := fmt.Sprintf("/tmp/ns.%s.%s", u.Name, disp)
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
