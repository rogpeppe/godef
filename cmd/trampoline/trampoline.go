package main

import (
	"code.google.com/p/rog-go/loopback"
	"flag"
	"fmt"
	"io"
	"os"
)

var localNet = flag.String("i", "tcp", "network to listen on (accepts loopback options)")
var remoteNet = flag.String("r", "tcp", "network to dial (accepts loopback options)")
var useStdin = flag.Bool("s", false, "use stdin and stdout instead of listening")

func fatalf(f string, a ...interface{}) {
	m := fmt.Sprintf(f, a...)
	fmt.Fprintf(os.Stderr, "%s\n", m)
	os.Exit(2)
}

type rw struct {
	io.Reader
	io.WriteCloser
}

func main() {
	flag.Parse()
	if *useStdin {
		if flag.NArg() != 1 {
			flag.Usage()
		}
		raddr := flag.Arg(0)
		transfer(rw{os.Stdin, os.Stdout}, raddr)
		return
	}

	if flag.NArg() != 2 {
		flag.Usage()
	}
	laddr := flag.Arg(0)
	raddr := flag.Arg(1)
	listener, err := loopback.Listen(*localNet, laddr)
	if err != nil {
		fatalf("listen: %v", err)
	}
	for {
		c, err := listener.Accept()
		if err != nil {
			fatalf("accept: %v", err)
		}
		go transfer(c, raddr)
	}
}

func copy(w io.WriteCloser, r io.Reader, done chan bool) {
	io.Copy(w, r)
	w.Close()
	done <- true
}

func transfer(c io.ReadWriteCloser, raddr string) {
	rc, err := loopback.Dial(*remoteNet, "", raddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial: %v\n", err)
		return
	}
	done := make(chan bool)
	go copy(c, rc, done)
	go copy(rc, c, done)
	<-done
	<-done
}
