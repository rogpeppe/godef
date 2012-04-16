package main

// An interactive client for 9P servers.

import (
	"bufio"
	"code.google.com/p/rog-go/go9p/g9p"
	"code.google.com/p/rog-go/go9p/g9pc"
	"code.google.com/p/rog-go/go9p/g9plog"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
)

var addr = flag.String("addr", "127.0.0.1:5640", "network address")
var ouser = flag.String("user", "", "user to connect as")
var cmdfile = flag.String("file", "", "read commands from file")
var prompt = flag.String("prompt", "9p> ", "prompt for interactive client")
var debug = flag.Bool("d", false, "enable debugging (fcalls)")
var debugall = flag.Bool("D", false, "enable debugging (raw packets)")
var serveHTTP = flag.String("h", "", "serve HTTP logging requests on the given network address")

var cwd = "/"
var cfid *g9pc.Fid

type Cmd struct {
	fun  func(c *g9pc.Client, s []string)
	help string
}

var cmds map[string]*Cmd

func init() {
	cmds = make(map[string]*Cmd)
	cmds["write"] = &Cmd{cmdwrite, "write file string [...]\t«write the unmodified string to file, create file if necessary»"}
	cmds["echo"] = &Cmd{cmdecho, "echo file string [...]\t«echo string to file (newline appended)»"}
	cmds["stat"] = &Cmd{cmdstat, "stat file [...]\t«stat file»"}
	cmds["ls"] = &Cmd{cmdls, "ls [-l] file [...]\t«list contents of directory or file»"}
	cmds["cd"] = &Cmd{cmdcd, "cd dir\t«change working directory»"}
	cmds["cat"] = &Cmd{cmdcat, "cat file [...]\t«print the contents of file»"}
	cmds["mkdir"] = &Cmd{cmdmkdir, "mkdir dir [...]\t«create dir on remote server»"}
	cmds["get"] = &Cmd{cmdget, "get file [local]\t«get file from remote server»"}
	cmds["put"] = &Cmd{cmdput, "put file [remote]\t«put file on the remote server as 'file'»"}
	cmds["pwd"] = &Cmd{cmdpwd, "pwd\t«print working directory»"}
	cmds["rm"] = &Cmd{cmdrm, "rm file [...]\t«remove file from remote server»"}
	cmds["help"] = &Cmd{cmdhelp, "help [cmd]\t«print available commands or help on cmd»"}
	cmds["quit"] = &Cmd{cmdquit, "quit\t«exit»"}
	cmds["exit"] = &Cmd{cmdquit, "exit\t«quit»"}
}

// normalize user-supplied path. path starting with '/' is left untouched, otherwise is considered
// local from cwd
func normpath(s string) string {
	if s[0] == '/' {
		return path.Clean(s)
	}
	return path.Clean(cwd + "/" + s)
}

func b(mode uint32, s uint8) string {
	var bits = []string{"---", "--x", "-w-", "-wx", "r--", "r-x", "rw-", "rwx"}
	return bits[(mode>>s)&7]
}

// Convert file mode bits to string representation
func modetostr(mode uint32) string {
	d := "-"
	if mode&g9p.DMDIR != 0 {
		d = "d"
	} else if mode&g9p.DMAPPEND != 0 {
		d = "a"
	}
	return fmt.Sprintf("%s%s%s%s", d, b(mode, 6), b(mode, 3), b(mode, 0))
}

// Write the string s to remote file f. Create f if it doesn't exist
func writeone(c *g9pc.Client, f, s string) {
	fname := normpath(f)
	file, err := c.FOpen(fname, g9p.OWRITE|g9p.OTRUNC)
	if err != nil {
		file, err = c.FCreate(fname, 0666, g9p.OWRITE)
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
func cmdwrite(c *g9pc.Client, s []string) {
	fname := normpath(s[0])
	str := strings.Join(s[1:], " ")
	writeone(c, fname, str)
}

// Echo (append newline) s[1:] to s[0]
func cmdecho(c *g9pc.Client, s []string) {
	fname := normpath(s[0])
	str := strings.Join(s[1:], " ") + "\n"
	writeone(c, fname, str)
}

// Stat the remote file f
func statone(c *g9pc.Client, f string) {
	fname := normpath(f)

	stat, err := c.FStat(fname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in stat %s: %s\n", fname, err)
		return
	}
	fmt.Fprintf(os.Stdout, "%s\n", stat)
}

func cmdstat(c *g9pc.Client, s []string) {
	for _, f := range s {
		statone(c, normpath(f))
	}
}

func dirtostr(d *g9p.Dir) string {
	return fmt.Sprintf("%s %s %s %-8d\t\t%s", modetostr(d.Mode), d.Uid, d.Gid, d.Length, d.Name)
}

func lsone(c *g9pc.Client, s string, long bool) {
	st, err := c.FStat(normpath(s))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error stat: %s\n", err)
		return
	}
	if st.Mode&g9p.DMDIR != 0 {
		file, err := c.FOpen(s, g9p.OREAD)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening dir: %s\n", err)
			return
		}
		defer file.Close()
		for {
			d, err := file.Readdir(0)
			if err != nil {
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

func cmdls(c *g9pc.Client, s []string) {
	long := false
	if len(s) > 0 && s[0] == "-l" {
		long = true
		s = s[1:]
	}
	if len(s) == 0 {
		lsone(c, cwd, long)
	} else {
		for _, d := range s {
			lsone(c, cwd+d, long)
		}
	}
}

func walkone(c *g9pc.Client, s string) {
	ncwd := normpath(s)
	_, err := c.FWalk(ncwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk error: %s\n", err)
		return
	}
	cwd = ncwd
}

func cmdcd(c *g9pc.Client, s []string) {
	if s != nil {
		walkone(c, strings.Join(s, "/"))
	}
}

// Print the contents of f
func cmdcat(c *g9pc.Client, s []string) {
	buf := make([]byte, 8192)
	for _, f := range s {
		fname := normpath(f)
		file, err := c.FOpen(fname, g9p.OREAD)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening %s: %s\n", f, err)
			continue
		}
		defer file.Close()
		for {
			n, err := file.Read(buf)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading %s: %s\n", f, err)
			}
			if n == 0 {
				break
			}
			os.Stdout.Write(buf[0:n])
		}
	}
}

// Create a single directory on remote server
func mkone(c *g9pc.Client, s string) {
	fname := normpath(s)
	file, err := c.FCreate(fname, 0777|g9p.DMDIR, g9p.OWRITE)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating directory %s: %s\n", fname, err)
		return
	}
	file.Close()
}

// Create directories on remote server
func cmdmkdir(c *g9pc.Client, s []string) {
	for _, f := range s {
		mkone(c, f)
	}
}

// Copy a remote file to local filesystem
func cmdget(c *g9pc.Client, s []string) {
	var from, to string
	switch len(s) {
	case 1:
		from = normpath(s[0])
		_, to = path.Split(s[0])
	case 2:
		from, to = normpath(s[0]), s[1]
	default:
		fmt.Fprintf(os.Stderr, "from arguments; usage: get from to\n")
	}

	tofile, err := os.Create(to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
		return
	}
	defer tofile.Close()

	file, ferr := c.FOpen(from, g9p.OREAD)
	if ferr != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
		return
	}
	defer file.Close()

	buf := make([]byte, 8192)
	for {
		n, oserr := file.Read(buf)
		if oserr != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %s\n", from, oserr)
			return
		}
		if n == 0 {
			break
		}

		m, err := tofile.Write(buf[0:n])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %s\n", to, err)
			return
		}

		if m != n {
			fmt.Fprintf(os.Stderr, "short write %s\n", to)
			return
		}
	}
}

// Copy a local file to remote server
func cmdput(c *g9pc.Client, s []string) {
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

	file, ferr := c.FOpen(to, g9p.OWRITE|g9p.OTRUNC)
	if ferr != nil {
		file, ferr = c.FCreate(to, 0666, g9p.OWRITE)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
			return
		}
	}
	defer file.Close()

	buf := make([]byte, 8192)
	for {
		n, oserr := fromfile.Read(buf)
		if oserr != nil && oserr != io.EOF {
			fmt.Fprintf(os.Stderr, "error reading %s: %s\n", from, oserr)
			return
		}

		if n == 0 {
			break
		}

		m, err := file.Write(buf[0:n])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %s\n", to, err)
			return
		}

		if m != n {
			fmt.Fprintf(os.Stderr, "short write %s\n", to)
			return
		}
	}
}

func cmdpwd(c *g9pc.Client, s []string) { fmt.Fprintf(os.Stdout, cwd+"\n") }

// Remove f from remote server
func rmone(c *g9pc.Client, f string) {
	fname := normpath(f)

	err := c.FRemove(fname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in stat %s: %s\n", fname, err)
		return
	}
}

// Remove one or more files from the server
func cmdrm(c *g9pc.Client, s []string) {
	for _, f := range s {
		rmone(c, normpath(f))
	}
}

// Print available commands
func cmdhelp(c *g9pc.Client, s []string) {
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

func cmdquit(c *g9pc.Client, s []string) { os.Exit(0) }

func cmd(c *g9pc.Client, cmd string) {
	ncmd := strings.Fields(cmd)
	if len(ncmd) <= 0 {
		return
	}
	v, ok := cmds[ncmd[0]]
	if ok == false {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", ncmd[0])
		return
	}
	v.fun(c, ncmd[1:])
	return
}

func interactive(c *g9pc.Client) {
	reader, ok := bufio.NewReaderSize(os.Stdin, 8192)
	if ok != nil {
		fmt.Fprintf(os.Stderr, "can't create reader buffer: %s\n", ok)
	}
	done := make(chan error)
	go func() {
		e := c.Wait()
		fmt.Printf("wait finished (%v)\n", e)
		done <- e
	}()
loop:
	for {
		select {
		case e := <-done:
			fmt.Fprintf(os.Stderr, "server: %v\n", e)
			break loop
		default:
		}
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
				cmd(c, in[i])
			}
		}
	}
}

func main() {
	var user g9p.User
	var file *g9pc.File

	flag.Parse()

	if *ouser == "" {
		user = g9p.OsUsers.Uid2User(os.Geteuid())
	} else {
		user = g9p.OsUsers.Uname2User(*ouser)
	}

	naddr := *addr
	if strings.LastIndex(naddr, ":") == -1 {
		naddr = naddr + ":5640"
	}

	var plog g9p.Logger
	if *serveHTTP != "" {
		plog = g9plog.NewClient(naddr, -1, 0)
		go func() {
			if err := http.ListenAndServe(*serveHTTP, nil); err != nil {
				log.Exitln("http listen: ", err)
			}
		}()
	}

	c, err := g9pc.Mount("tcp", naddr, "", user, plog)
	if err != nil {
		log.Exitln("error mounting %s: %s", naddr, err)
	}

	//	if *debug {
	//		c.Debuglevel = 1
	//	}
	//	if *debugall {
	//		c.Debuglevel = 2
	//	}

	walkone(c, "/")

	if file != nil {
		//process(c)
		fmt.Sprint(os.Stderr, "file reading unimplemented\n")
	} else if flag.NArg() > 0 {
		flags := flag.Args()
		for _, uc := range flags {
			cmd(c, uc)
		}
	} else {
		interactive(c)
	}

	return
}
