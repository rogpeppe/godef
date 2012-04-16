package main

import (
	"code.google.com/p/rog-go/go9p/g9p"
	"code.google.com/p/rog-go/go9p/g9pc"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

var debuglevel = flag.Int("d", 0, "debuglevel")
var addr = flag.String("addr", "127.0.0.1:5640", "network address")

func main() {
	var m int
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

	file, err = c.FOpen(flag.Arg(0), g9p.OWRITE|g9p.OTRUNC)
	if err != nil {
		file, err = c.FCreate(flag.Arg(0), 0666, g9p.OWRITE)
		if err != nil {
			goto error
		}
	}

	buf := make([]byte, 8192)
	for {
		n, oserr := os.Stdin.Read(buf)
		if oserr != nil && oserr != io.EOF {
			err = &g9p.Error{oserr.String(), 0}
			goto error
		}

		if n == 0 {
			break
		}

		m, err = file.Write(buf[0:n])
		if err != nil {
			goto error
		}

		if m != n {
			err = &g9p.Error{"short write", 0}
			goto error
		}
	}

	file.Close()
	return

error:
	log.Stderr(fmt.Sprintf("Error: %v", err))
}
