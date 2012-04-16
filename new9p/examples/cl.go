package main

// An interactive client for 9P servers.

import (
	"bufio"
	g9p "code.google.com/p/rog-go/new9p"
	g9pc "code.google.com/p/rog-go/new9p/client"
	"code.google.com/p/rog-go/new9p/seq"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
)

var addr = flag.String("addr", "127.0.0.1:5640", "network address")
var ouser = flag.String("user", "", "user to connect as")
var cmdfile = flag.String("file", "", "read commands from file")
var prompt = flag.String("prompt", "9p> ", "prompt for interactive client")

var cwd = "/"

type Cmd struct {
	fun  func(ns *g9pc.Ns, s []string)
	help string
}

var cmds map[string]*Cmd

func init() {
	cmds = map[string]*Cmd{
		"write":   &Cmd{cmdwrite, "write file string [...]\t«write the unmodified string to file, create file if necessary»"},
		"echo":    &Cmd{cmdecho, "echo file string [...]\t«echo string to file (newline appended)»"},
		"stat":    &Cmd{cmdstat, "stat file [...]\t«stat file»"},
		"ls":      &Cmd{cmdls, "ls [-l] file [...]\t«list contents of directory or file»"},
		"cd":      &Cmd{cmdcd, "cd dir\t«change working directory»"},
		"cat":     &Cmd{cmdcat, "cat file [...]\t«print the contents of file»"},
		"stream":  &Cmd{cmdstream, "stream file [...]\t«print the contents of file by streaming»"},
		"mkdir":   &Cmd{cmdmkdir, "mkdir dir [...]\t«create dir on remote server»"},
		"get":     &Cmd{cmdget, "get file [local]\t«get file from remote server»"},
		"put":     &Cmd{cmdput, "put file [remote]\t«put file on the remote server as 'file'»"},
		"pwd":     &Cmd{cmdpwd, "pwd\t«print working directory»"},
		"rm":      &Cmd{cmdrm, "rm file [...]\t«remove file from remote server»"},
		"help":    &Cmd{cmdhelp, "help [cmd]\t«print available commands or help on cmd»"},
		"torture": &Cmd{cmdtorture, "torture [dir]\t«torture»"},
		"read":    &Cmd{cmdread, "read file"},
		"quit":    &Cmd{cmdquit, "quit\t«exit»"},
		"exit":    &Cmd{cmdquit, "exit\t«quit»"},
	}
}

func normpath(s string) string {
	return path.Clean(s)
}

func b(mode uint32, s uint8) string {
	var bits = []string{"---", "--x", "-w-", "-wx", "r--", "r-x", "rw-", "rwx"}
	return bits[(mode>>s)&7]
}

// Write the string s to remote file f. Create f if it doesn't exist
func writeone(ns *g9pc.Ns, fname, s string) {
	file, err := ns.Open(fname, g9p.OWRITE|g9p.OTRUNC)
	if err != nil {
		file, err = ns.Create(fname, g9p.OWRITE, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening %s: %v\n", fname, err)
			return
		}
	}
	defer file.Close()

	m, err := file.Write([]byte(s))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing to %s: %s\n", fname, err)
		return
	}

	if m != len(s) {
		fmt.Fprintf(os.Stderr, "short write %s\n", fname)
		return
	}
}

// Write s[1:] (with appended spaces) to the file s[0]
func cmdwrite(ns *g9pc.Ns, s []string) {
	writeone(ns, s[0], strings.Join(s[1:], " "))
}

// Echo (append newline) s[1:] to s[0]
func cmdecho(ns *g9pc.Ns, s []string) {
	writeone(ns, s[0], strings.Join(s[1:], " ")+"\n")
}

// Stat the remote file f
func statone(ns *g9pc.Ns, f string) {
	stat, err := ns.Stat(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in stat %s: %s\n", f, err)
		return
	}
	fmt.Fprintf(os.Stdout, "%v\n", stat)
}

func cmdstat(ns *g9pc.Ns, s []string) {
	for _, f := range s {
		statone(ns, f)
	}
}

func dirtostr(d *g9p.Dir) string {
	return fmt.Sprintf("%v %s %s %-8d\t\t%s", d.Mode, d.Uid, d.Gid, d.Length, d.Name)
}

func lsone(ns *g9pc.Ns, s string, long bool) {
	st, err := ns.Stat(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error stat: %s\n", err)
		return
	}
	if st.Mode&g9p.DMDIR != 0 {
		file, err := ns.Open(s, g9p.OREAD)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening dir: %s\n", err)
			return
		}
		defer file.Close()
		for {
			d, err := file.Dirread()
			if err != nil && err != io.EOF {
				fmt.Fprintf(os.Stderr, "error reading dir: %s\n", err)
			}
			if d == nil || len(d) == 0 {
				break
			}
			for _, dir := range d {
				if long {
					fmt.Fprintf(os.Stdout, "%s\n", dirtostr(dir))
				} else {
					os.Stdout.WriteString(dir.Name + "\n")
				}
			}
		}
	} else {
		fmt.Fprintf(os.Stdout, "%s\n", dirtostr(st))
	}
}

func cmdls(ns *g9pc.Ns, s []string) {
	long := false
	if len(s) > 0 && s[0] == "-l" {
		long = true
		s = s[1:]
	}
	if len(s) == 0 {
		lsone(ns, ".", long)
	} else {
		for _, d := range s {
			lsone(ns, d, long)
		}
	}
}

func cmdcd(ns *g9pc.Ns, s []string) {
	if s == nil {
		return
	}
	d := s[0]
	err := ns.Chdir(d)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chdir: %v\n", err)
		return
	}
	cwd = path.Clean(cwd + "/" + d)
}

// Print the contents of f
func cmdcat(ns *g9pc.Ns, s []string) {
	for _, fname := range s {
		file, err := ns.Open(fname, g9p.OREAD)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening %s: %s\n", fname, err)
			continue
		}
		defer file.Close()
		_, err = io.Copy(os.Stdout, file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
}

// Print the contents of f using streaming.
func cmdstream(ns *g9pc.Ns, s []string) {
	for _, fname := range s {
		r := ns.ReadStream(fname, 20, 10)
		_, err := io.Copy(os.Stdout, r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", fname, err)
		}
		r.Close()
	}
}

// Create a single directory on remote server
func mkone(ns *g9pc.Ns, fname string) {
	file, err := ns.Create(fname, g9p.OREAD, 0777|g9p.DMDIR)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating directory %s: %s\n", fname, err)
		return
	}
	file.Close()
}

// Create directories on remote server
func cmdmkdir(ns *g9pc.Ns, s []string) {
	for _, f := range s {
		mkone(ns, f)
	}
}

// Copy a remote file to local filesystem
func cmdget(ns *g9pc.Ns, s []string) {
	var from, to string
	switch len(s) {
	case 1:
		from, to = path.Split(path.Clean(s[0]))
	case 2:
		from, to = s[0], s[1]
	default:
		fmt.Fprintf(os.Stderr, "from arguments; usage: get from to\n")
	}

	tofile, err := os.Create(to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
		return
	}
	defer tofile.Close()

	file, err := ns.Open(from, g9p.OREAD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
		return
	}
	defer file.Close()

	_, err = io.Copy(tofile, file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error copying: %v\n", err)
		return
	}
}

// Copy a local file to remote server
func cmdput(ns *g9pc.Ns, s []string) {
	var from, to string
	switch len(s) {
	case 1:
		_, to = path.Split(s[0])
		to = normpath(to)
		from = s[0]
	case 2:
		from, to = s[0], normpath(s[1])
	default:
		fmt.Fprintf(os.Stderr, "incorrect arguments; usage: put local [remote]\n")
	}

	fromfile, err := os.Open(from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for reading: %s\n", from, err)
		return
	}
	defer fromfile.Close()

	file, err := ns.Open(to, g9p.OWRITE|g9p.OTRUNC)
	if err != nil {
		file, err = ns.Create(to, g9p.OWRITE, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
			return
		}
	}
	fmt.Fprintf(os.Stderr, "opened file ok\n")
	defer file.Close()
	n, err := Copy(file, fromfile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error copying file: %v (%d bytes copied)\n", err, n)
	}
	fmt.Fprintf(os.Stderr, "copied %d bytes\n", n)
}

func cmdpwd(ns *g9pc.Ns, s []string) {
	fmt.Fprintf(os.Stdout, "%s\n", cwd)
}

// Remove f from remote server
func rmone(ns *g9pc.Ns, f string) {
	err := ns.Remove(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in stat %s: %s\n", f, err)
		return
	}
}

// Remove one or more files from the server
func cmdrm(ns *g9pc.Ns, s []string) {
	for _, f := range s {
		rmone(ns, f)
	}
}

// Print available commands
func cmdhelp(ns *g9pc.Ns, s []string) {
	cmdstr := ""
	if len(s) > 0 {
		for _, h := range s {
			v, ok := cmds[h]
			if ok {
				cmdstr = cmdstr + v.help + "\n"
			} else {
				cmdstr = cmdstr + "unknown command: " + h + "\n"
			}
		}
	} else {
		cmdstr = "available commands: "
		for k, _ := range cmds {
			cmdstr = cmdstr + " " + k
		}
		cmdstr = cmdstr + "\n"
	}
	fmt.Fprintf(os.Stdout, "%s", cmdstr)
}

type traverser struct {
	out  chan string
	refc chan int
	tokc chan bool
}

func cmdtorture(ns *g9pc.Ns, s []string) {
	path := "."
	if len(s) > 0 {
		path = s[0]
	}
	t := &traverser{
		out:  make(chan string),
		refc: make(chan int),
		//		tokc: make(chan bool, 2),
	}
	if len(s) > 1 {
		max, err := strconv.Atoi(s[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "max?: %v\n", err)
			return
		}
		t.tokc = make(chan bool, max)
		for i := 0; i < max; i++ {
			t.tokc <- true
		}
	}
	fid, err := ns.Walk(path)
	if fid == nil {
		fmt.Fprintf(os.Stderr, "cannot walk to %s: %v\n", s[0], err)
		return
	}
	ref := 1
	maxref := 1
	go func() {
		t.traverse(fid, path, "", make(chan bool, 1))
		t.refc <- -1
	}()
	for ref > 0 {
		select {
		case s := <-t.out:
			fmt.Print("************ ", s)
		case r := <-t.refc:
			ref += r
			if ref > maxref {
				maxref = ref
			}
		}
	}
	fmt.Printf("\n")
	fmt.Printf("max procs %d\n", maxref)
}

func (t *traverser) traverse(parent *g9pc.NsFile, path, name string, sync chan bool) {
	sq, results := seq.NewSequencer()
	doneWalk := make(chan bool)
	go func() {
		defer close(doneWalk)
		if name != "" {
			_, ok := <-results // SeqWalk
			if !ok {
				t.printf("cannot walk to %q: %v", path+"/"+name, sq.Error())
				return
			}
		}
		doneWalk <- true
		<-results          // readDir or readFile.
		_, ok := <-results // eof.
		if ok {
			panic("expected closed")
		}
	}()
	fid := parent
	if name != "" {
		fid = parent.SeqWalk(sq, name)
	}
	_, ok := <-doneWalk
	if !ok {
		return
	}
	sync <- true
	t.printf("read %q, dir %v", path+"/"+name, fid.IsDir())
	t.refc <- 1
	go func() {
		t.grab()
		path += "/" + name
		if fid.IsDir() {
			t.readDir(sq, fid, path)
		} else {
			t.readFile(sq, fid, path)
		}
		sq.Do(nil, nil)
		t.refc <- -1
		t.release()
	}()
}

func (t *traverser) readDir(pseq *seq.Sequencer, fid *g9pc.NsFile, path string) {
	t.printf("readDir %s", path)
	sq, results := pseq.Subsequencer("readDir")
	errc := make(chan error, 1)
	go func() {
		<-results          // SeqWalk (clone)
		_, ok := <-results // OpenReq
		if !ok {
			errc <- fmt.Errorf("cannot open %q: %#v", path, sq.Error())
			return
		}
		<-results // NonseqReq
		<-results // ReadStream
		errc <- nil
		_, ok = <-results // eof
		if ok {
			panic("expected closed")
		}
		errc <- nil
	}()

	rfid := fid.SeqWalk(sq)
	//	defer rfid.Close()		TODO something better!

	sq.Do(rfid.File(), seq.OpenReq{g9p.OREAD})
	sq.Do(fid.File(), seq.NonseqReq{})
	rd := rfid.SeqReadStream(sq, 5, 8192)
	defer rd.Close()

	buf, _ := ioutil.ReadAll(rd)
	t.printf("read %d bytes from %q", len(buf), path)
	err := <-errc
	sq.Do(nil, nil)
	<-errc
	//we get here but fid still can be part of the sequence.
	//maybe that means that subsequence has not terminated
	//correctly. no it doesn't. it means that the overall sequence
	//has not terminated correctly.
	//
	//question: should files opened as part of a subsequence be
	//ratified by the subsequence finishing?
	//only 

	if err != nil && len(buf) == 0 {
		sq.Result(nil, err)
		t.printf("error on %s: %v\n", path, err)
		return
	}

	d, err := g9p.UnmarshalDirs(buf)
	if err != nil {
		t.printf("cannot unpack directory %s: %v\n", path, err)
		return
	}

	sync := make(chan bool)
	for i, dir := range d {
		t.printf("%q[%d]: %v", path, i, dir)
		go t.traverse(fid, path, dir.Name, sync)
	}
	for i := 0; i < len(d); i++ {
		<-sync
	}
	t.printf("%s: %d entries", path, len(d))
	sq.Result(seq.StringResult("readDir"), nil)
}

func (t *traverser) printf(f string, args ...interface{}) {
	if f != "" && f[len(f)-1] != '\n' {
		f += "\n"
	}
	t.out <- fmt.Sprintf(f, args...)
}

func (t *traverser) grab() {
	if t.tokc != nil {
		<-t.tokc
	}
}

func (t *traverser) release() {
	if t.tokc != nil {
		t.tokc <- true
	}
}

type nullWriter struct{}

func (nullWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (t *traverser) readFile(sq *seq.Sequencer, fid *g9pc.NsFile, path string) {
	t.printf("readFile %s", path)
	sq, results := sq.Subsequencer("readFile")
	go func() {
		_, ok := <-results // open
		if !ok {
			t.printf("cannot open %s: %v", path, sq.Error())
			return
		}
		<-results // stream
		_, ok = <-results
		if ok {
			panic("expected closed")
		}
		sq.Result(seq.StringResult("readFile"), sq.Error())
	}()
	sq.Do(fid.File(), seq.OpenReq{g9p.OREAD})
	rd := fid.SeqReadStream(sq, 20, 8192)
	tot, _ := io.Copy(nullWriter{}, rd)
	t.printf("%10d %s", tot, path)
	sq.Do(nil, nil)
}

func cmdquit(ns *g9pc.Ns, s []string) {
	os.Exit(0)
}

func cmdread(ns *g9pc.Ns, s []string) {
	sq, results := seq.NewSequencer()
	go func() {
		r := <-results // walk result
		r = <-results  // open result
		r = <-results  // readstream result
		_, ok := <-results
		if ok {
			panic("expected closed")
		}
	}()

	f := ns.SeqWalk(sq, s[0])
	sq.Do(f.File(), seq.OpenReq{g9p.OREAD})
	rd := f.SeqReadStream(sq, 200, 20)

	buf := make([]byte, 10)
	for {
		n, err := rd.Read(buf)
		if n == 0 {
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			break
		}
		fmt.Printf("%q\n", buf[0:n])
	}
	rd.Close()
	sq.Do(f.File(), seq.ClunkReq{}) // strictly speaking unnecessary.
	sq.Do(nil, nil)
	sq.Wait()
}

func cmd(ns *g9pc.Ns, cmd string) {
	ncmd := strings.Fields(cmd)
	if len(ncmd) <= 0 {
		return
	}
	v, ok := cmds[ncmd[0]]
	if ok == false {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", ncmd[0])
		return
	}
	v.fun(ns, ncmd[1:])
	return
}

func interactive(ns *g9pc.Ns) {
	reader, ok := bufio.NewReaderSize(os.Stdin, 8192)
	if ok != nil {
		fmt.Fprintf(os.Stderr, "can't create reader buffer: %s\n", ok)
	}
	for {
		fmt.Print(*prompt)
		line, ok := reader.ReadSlice('\n')
		if ok != nil {
			fmt.Fprintf(os.Stderr, "exiting...\n")
			break
		}
		str := strings.TrimSpace(string(line))
		// TODO: handle larger input lines by doubling buffer
		in := strings.Split(str, "\n")
		for i := range in {
			if len(in[i]) > 0 {
				cmd(ns, in[i])
			}
		}
	}
}

func init() {
	log.SetFlags(log.Lmicroseconds) //  | log.Lshortfile
	//	log.SetOutput(nullWriter{})
}

func main() {
	flag.Parse()

	naddr := *addr
	if strings.LastIndex(naddr, ":") == -1 {
		naddr = naddr + ":5640"
	}

	c, err := g9pc.Mount("tcp", naddr, "")
	if err != nil {
		log.Fatalf("error mounting %s: %v", naddr, err)
	}

	ns := new(g9pc.Ns)
	root, err := c.Walk("")
	if err != nil {
		log.Fatalf("error walking to /: %v", err)
	}
	ns.Root = g9pc.NewNsFile(root.File())
	ns.Dot = ns.Root

	if flag.NArg() > 0 {
		flags := flag.Args()
		for _, uc := range flags {
			cmd(ns, uc)
		}
	} else {
		interactive(ns)
	}

	return
}

func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	// If the writer has a ReadFrom method, use it to to do the copy.
	// Avoids an allocation and a copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}
	// Similarly, if the reader has a WriteTo method, use it to to do the copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return written, err
}
