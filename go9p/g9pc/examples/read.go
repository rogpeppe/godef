package main

import (
	"code.google.com/p/rog-go/go9p/g9p"
	"code.google.com/p/rog-go/go9p/g9pc"
	"flag"
	"fmt"
	"log"
	"os"
)

var debuglevel = flag.Int("d", 0, "debuglevel")
var addr = flag.String("addr", "127.0.0.1:5640", "network address")

func main() {
	var n int
	var user g9p.User
	var file *g9pc.File

	flag.Parse()
	user = g9p.OsUsers.Uid2User(os.Geteuid())
	c, err := g9pc.Mount("tcp", *addr, "", user, nil)
	if err != nil {
		goto error
	}

	if flag.NArg() != 1 {
		log.Stderr("invalid arguments")
		return
	}

	file, err = c.FOpen(flag.Arg(0), g9p.OREAD)
	if err != nil {
		goto error
	}

	buf := make([]byte, 8192)
	for {
		n, err = file.Read(buf)
		if err != nil {
			goto error
		}

		if n == 0 {
			break
		}

		os.Stdout.Write(buf[0:n])
	}

	file.Close()
	return

error:
	log.Stderr(fmt.Sprintf("Error: %v", err))
}
