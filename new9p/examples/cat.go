package main

import (
	"fmt"
	"io"
	"os"

	"code.google.com/p/goplan9/plan9"
	"code.google.com/p/goplan9/plan9/client"
)

func main() {
	fsys, err := client.MountService("acme")
	if err != nil {
		panic(err)
	}

	fid, err := fsys.Open("index", plan9.OREAD)
	if err != nil {
		panic(err)
	}

	fid.Write([]byte("hello, world"))

	io.Copy(os.Stdout, fid)
	fid.Close()

	d, err := fsys.Stat("/index")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%v\n", d)

	fsys.Wstat("/index", d)
}
