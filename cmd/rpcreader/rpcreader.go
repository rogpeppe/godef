// rpcreader demonstrates using RPC to initiate file streaming
// over the same connection.
//
// Start a server instance with:
//
//     rpcreader -s addr
//
// Start a client with:
//
//     rpcreader addr
//
// The client is interactive. There's only one command: read,
// which takes a list of filename arguments and streams
// their data to the client.
package main

import (
	"bufio"
	"code.google.com/p/rog-go/ncrpc"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"netchan"
	"os"
	"strings"
	"sync"
)

var server = flag.Bool("s", false, "server mode")

type Server struct {
	mu     sync.Mutex
	chanId int
	exp    *netchan.Exporter
}

type ReadReq struct {
	Paths []string
}

type ReadResponse struct {
	Info  []struct{ X os.FileInfo }
	Error []string
	Chan  string // name of channel to receive data on.
}

func openFile(path string) (fd *os.File, info os.FileInfo, err error) {
	fd, err = os.Open(path)
	if fd == nil {
		return
	}
	info, err = fd.Stat()
	if info.IsDir() {
		fd.Close()
		fd = nil
		info = nil
		err = errors.New("cannot read directory")
	}
	return
}

func (srv *Server) Read(req *ReadReq, resp *ReadResponse) error {
	srv.mu.Lock()
	id := srv.chanId
	srv.chanId++
	srv.mu.Unlock()

	n := len(req.Paths)
	fd := make([]*os.File, n)
	resp.Info = make([]struct{ X os.FileInfo }, n)
	resp.Error = make([]string, n)
	for i, path := range req.Paths {
		var err error
		fd[i], resp.Info[i].X, err = openFile(path)
		if err != nil {
			resp.Error[i] = err.Error()
		}
	}
	resp.Chan = fmt.Sprintf("data%d", id)
	data := make(chan []byte)

	if err := srv.exp.Export(resp.Chan, data, netchan.Send); err != nil {
		return err
	}
	go func() {
		for _, f := range fd {
			if f == nil {
				continue
			}
			for {
				buf := make([]byte, 8192)
				n, err := f.Read(buf)
				if n > 0 {
					data <- buf[0:n]
				}
				if err != nil {
					break
				}
			}
			data <- nil
			f.Close()
		}
		srv.exp.Hangup(resp.Chan)
	}()
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: rpcreader [-s] tcp-addr\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		return
	}
	addr := flag.Arg(0)
	if *server {
		srv, err := ncrpc.NewServer(false)
		if err != nil {
			log.Fatal("ncrpc NewServer failed: ", err)
		}
		srv.RPCServer.Register(&Server{exp: srv.Exporter})
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatal("listen failed: ", err)
		}
		srv.Exporter.Serve(lis)
		return
	}
	client, err := ncrpc.Import("tcp", addr)
	if err != nil {
		log.Fatal("dial failed: ", err)
	}
	interact(client)
}

type command struct {
	narg int
	f    func(client *ncrpc.Client, args []string) error
}

var commands = map[string]command{
	"read": {-1, readcmd},
}

func interact(client *ncrpc.Client) {
	stdin := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, "> ")
		line, err := stdin.ReadString('\n')
		if err != nil {
			break
		}
		args := strings.Fields(line)
		if len(args) == 0 {
			continue
		}
		cmd := commands[args[0]]
		if cmd.f == nil {
			fmt.Printf("unknown command\n")
			continue
		}
		if cmd.narg >= 0 && cmd.narg != len(args)-1 {
			fmt.Printf("invalid argument count\n")
			continue
		}
		err = cmd.f(client, args[1:])
		if err != nil {
			fmt.Printf("failure: %v\n", err)
		}
	}
}

func readcmd(client *ncrpc.Client, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	var resp ReadResponse
	err := client.Server.Call("Server.Read", &ReadReq{Paths: paths}, &resp)
	if err != nil {
		return fmt.Errorf("call: %v", err)
	}
	data := make(chan []byte)
	err = client.Importer.Import(resp.Chan, data, netchan.Recv, 50)
	if err != nil {
		return fmt.Errorf("import: %v", err)
	}
	for i, info := range resp.Info {
		if resp.Error[i] != "" {
			fmt.Printf("%s failed: %s\n", paths[i], resp.Error[i])
			continue
		}

		fmt.Printf("%s %03o %d\n", paths[i], info.X.Mode, info.X.Size)
		tot := int64(0)
		for {
			x := <-data
			if len(x) == 0 {
				break
			}
			tot += int64(len(x))
		}
		fmt.Printf("\tread %d bytes\n", tot)
	}
	client.Importer.Hangup(resp.Chan)
	return nil
}
