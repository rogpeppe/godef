/*
The pxargs command is a simpler version of xargs(1) that
can execute commands in parallel. It reads lines from
standard input and executes the command with the
lines as arguments. Flags determine the maximum
number of arguments to give to the command and
the maximum number of commands to run concurrently.

Unlike xargs, it recognises no metacharacters other
than newline.
*/
package main
import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var n = flag.Int("n", 300, "maximum number of arguments to pass")
var s = flag.Int("s", 100 * 1024, "maximum argument size")
var p = flag.Int("p", 1, "max number of commands to run concurrently")
var v = flag.Bool("v", false, "print commands as they're executed")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: pxargs [flags] command [arg...]\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if *n <= 0 {
		flag.Usage()
	}
	cmdArgs := flag.Args()
	if len(cmdArgs) == 0 {
		flag.Usage()
	}
	path, err := exec.LookPath(cmdArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "pxargs: %v\n", err)
		os.Exit(1)
	}
	cmdArgs[0] = path
	cmdArgSize := 0
	for _, a := range cmdArgs {
		cmdArgSize += len(a)
	}

	runc := make(chan []string)
	var wg sync.WaitGroup
	for i := 0; i < *p; i++ {
		wg.Add(1)
		go func() {
			runner(runc)
			wg.Done()
		}()
	}
	
	r := bufio.NewReader(os.Stdin)
	args := append([]string(nil), cmdArgs...)
	size := cmdArgSize
	for {
		l, err := r.ReadString('\n')
		if err != nil {
			break
		}
		if l[len(l)-1] == '\n' {
			l = l[0:len(l)-1]
		}
		args = append(args, l)
		size += len(l)
		if len(args) - len(cmdArgs) >= *n || size >= *s {
			if *v {
				fmt.Println(strings.Join(args, " "))
			}
			runc <- args
			args = append([]string(nil), cmdArgs...)
			size = cmdArgSize
		}
	}
	if len(args) > len(cmdArgs) {
		runc <- args
	}
	close(runc)
	wg.Wait()
}

func runner(runc <-chan []string) {
	for args := range runc {
		c := exec.Command(args[0], args[1:]...)
		os.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Run()
	}
}
