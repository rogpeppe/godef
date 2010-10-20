package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"rog-go.googlecode.com/hg/go9p/g9p"
	"rog-go.googlecode.com/hg/go9p/g9pc"
)

var debuglevel = flag.Int("d", 0, "debuglevel")
var addr = flag.String("addr", "127.0.0.1:5640", "network address")

func main() {
	var user g9p.User
	var err os.Error
	var c *g9pc.Client
	var file *g9pc.File
	var d []*g9p.Dir

	flag.Parse()
	user = g9p.OsUsers.Uid2User(os.Geteuid())
	c, err = g9pc.Mount("tcp", *addr, "", user, nil)
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

	for {
		d, err = file.Readdir(0)
		if err != nil {
			goto error
		}

		if d == nil || len(d) == 0 {
			break
		}

		for i := 0; i < len(d); i++ {
			os.Stdout.WriteString(d[i].Name + "\n")
		}
	}

	file.Close()
	return

error:
	log.Stderr(fmt.Sprintf("Error: %v", err))
}
