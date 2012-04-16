// Share is a piece of demo code to illustrate the flexibility of the rpc and netchan
// packages. It requires one instance to be running in server mode on some network
// address addr, e.g. localhost:3456:
//
// 	share -s localhost:3456
//
// Then in other windows or on other machines, run some client instances:
//	share -name foo localhost:3456
//
// The name must be different for each client instance.
// When a client instance is running, there are two commands:
//	list
//		List all currently connected client names
//	read client filename
//		Ask the given client for the contents of the named file.
package main

import (
	"bufio"
	"code.google.com/p/rog-go/ncrpc"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
	"strings"
)

var server = flag.Bool("s", false, "server mode")
var clientName = flag.String("name", "", "client name")

type ReadReq struct {
	Client string
	Path   string
}

type Void struct{}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: share [options] tcp-addr\n")
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
		ncsrv, err := ncrpc.NewServer(true)
		ncsrv.RPCServer.Register(&Server{ncsrv})
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatal("listen failed: ", err)
		}
		ncsrv.Exporter.Serve(lis)
		return
	}

	client, err := ncrpc.Import("tcp", addr)
	if err != nil {
		log.Fatal("dial failed: ", err)
	}
	rpcsrv := rpc.NewServer()
	rpcsrv.Register(Client{})
	err = client.Serve(*clientName, rpcsrv)
	if err != nil {
		log.Fatal("client.Serve failed: ", err)
	}
	interact(client.Server)
}

type Server struct {
	ncsrv *ncrpc.Server
}

func (srv *Server) List(_ *Void, names *[]string) error {
	*names = srv.ncsrv.ClientNames()
	return nil
}

func (srv *Server) Read(req *ReadReq, data *[]byte) error {
	client := srv.ncsrv.Client(req.Client)
	if client == nil {
		return errors.New("unknown client")
	}
	return client.Call("Client.Read", &req.Path, data)
}

type Client struct{}

func (Client) Read(file *string, data *[]byte) (err error) {
	f, err := os.Open(*file)
	if err != nil {
		return err
	}
	*data, err = ioutil.ReadAll(f)
	return
}

type command struct {
	narg int
	f    func(srv *rpc.Client, args []string) error
}

var commands = map[string]command{
	"read": {2, readcmd},
	"list": {0, listcmd},
}

func interact(srv *rpc.Client) {
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
		if cmd.narg != len(args)-1 {
			fmt.Printf("invalid argument count\n")
			continue
		}
		err = cmd.f(srv, args[1:])
		if err != nil {
			fmt.Printf("failure: %v\n", err)
		}
	}
}

func readcmd(srv *rpc.Client, args []string) error {
	var data []byte
	err := srv.Call("Server.Read", &ReadReq{Client: args[0], Path: args[1]}, &data)
	if err != nil {
		return err
	}
	os.Stdout.Write(data)
	return nil
}

func listcmd(srv *rpc.Client, _ []string) error {
	var clients []string
	err := srv.Call("Server.List", &Void{}, &clients)
	if err != nil {
		return err
	}
	fmt.Printf("%q\n", clients)
	return nil
}
