package main

import (
	g9p "code.google.com/p/rog-go/new9p"
	g9pc "code.google.com/p/rog-go/new9p/client"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
)

var old = flag.Bool("old", false, "use old 9p operations")
var fs *g9pc.Fsys
var ns *g9pc.Ns
var sum = make(chan int64)

func main() {
	log.SetOutput(nullWriter{})
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: bundle addr dir")
		return
	}
	var err error
	fs, err = g9pc.Mount("tcp", flag.Arg(0), "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mount failed:", err)
		return
	}
	root := g9pc.NewNsFile(fs.Root.File())
	ns = &g9pc.Ns{Root: root, Dot: root}
	fmt.Println("mounted")
	dir := flag.Arg(1)
	if *old {
		go func() {
			oldwalk(dir)
			close(sum)
		}()
	} else {
		go func() {
			newwalk(dir, true)
			close(sum)
		}()
	}
	tot := int64(0)
	for n := range sum {
		tot += n
	}

	fmt.Printf("total: %d\n", tot)
}

func oldwalk(name string) {
	fid, err := fs.Open(name, g9p.OREAD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open %q: %v\n", name, err)
		return
	}
	defer fid.Close()
	if fid.Qid().Type&g9p.QTDIR != 0 {
		data, err := ioutil.ReadAll(fid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %d bytes from %q: %v", len(data), err)
			return
		}
		d, err := g9p.UnmarshalDirs(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot unpack directory %s: %v\n", name, err)
			return
		}
		for _, dir := range d {
			oldwalk(name + "/" + dir.Name)
		}
		return
	}
	sum <- count(fid)
}

func count(r io.Reader) (tot int64) {
	tot, _ = io.Copy(nullWriter{}, r)
	return
}

func newwalk(name string, isDir bool) {
	r := ns.ReadStream(name, 20, 8192)
	if isDir {
		data, err := ioutil.ReadAll(r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %d bytes from %q: %v", len(data), err)
			return
		}
		d, err := g9p.UnmarshalDirs(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot unpack directory %s: %v\n", name, err)
			return
		}
		for _, dir := range d {
			newwalk(name+"/"+dir.Name, dir.Qid.Type&g9p.QTDIR != 0)
		}
		return
	}
	sum <- count(r)
}

type nullWriter struct{}

func (nullWriter) Write(data []byte) (int, error) {
	return len(data), nil
}
