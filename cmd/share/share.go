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
	"code.google.com/p/rog-go/ncnet"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/rpc"
	"netchan"
	"os"
	"strings"
	"sync"
	"time"
)

var server = flag.Bool("s", false, "server mode")
var clientName = flag.String("name", "", "client name")

type Server struct {
	mu       sync.Mutex
	clients  map[string]*rpc.Client
	exp      *netchan.Exporter
	clientid int
}

type ReadReq struct {
	Client string
	Path   string
}

type Void struct{}

func (srv *Server) Publish(name *string, clientId *string) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.clients[*name] != nil {
		return errors.New("client name already exists")
	}
	*clientId = fmt.Sprintf("client%d", srv.clientid)
	srv.clientid++
	listener, err := ncnet.Listen(srv.exp, *clientId)
	if err != nil {
		return fmt.Errorf("cannot listen on netchan %q: %v", *clientId, err)
	}
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("error on ncnet.Accept(%q): %v", *clientId, err)
			return
		}
		listener.Close()
		client := rpc.NewClient(conn)
		err = client.Call("Client.Init", &Void{}, &Void{})
		if err != nil {
			log.Printf("error on init: %v", err)
			return
		}
		srv.mu.Lock()
		srv.clients[*name] = client
		srv.mu.Unlock()
		// when call completes, client has left.
		client.Call("Client.Wait", &Void{}, &Void{})
		srv.mu.Lock()
		delete(srv.clients, *name)
		srv.mu.Unlock()
	}()
	return nil
}

func (srv *Server) List(_ *Void, names *[]string) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for name := range srv.clients {
		*names = append(*names, name)
	}
	return nil
}

func (srv *Server) Read(req *ReadReq, data *[]byte) error {
	srv.mu.Lock()
	client := srv.clients[req.Client]
	srv.mu.Unlock()
	if client == nil {
		return errors.New("unknown client")
	}
	return client.Call("Client.Read", &req.Path, data)
}

type Client struct{}

func (Client) Init(*Void, *Void) error {
	return nil
}

// Wait blocks until the client is ready to leave.
// Currently that's forever.
func (Client) Wait(*Void, *Void) error {
	<-make(chan int)
	return nil
}

func (Client) Read(file *string, data *[]byte) (err error) {
	f, err := os.Open(*file)
	if err != nil {
		return err
	}
	*data, err = ioutil.ReadAll(f)
	return
}

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
		exp := netchan.NewExporter()
		if err := exp.ListenAndServe("tcp", addr); err != nil {
			log.Fatal("listen failed: ", err)
		}
		listener, err := ncnet.Listen(exp, "ctl")
		if err != nil {
			log.Fatal("ncnet listen failed: ", err)
		}
		srv := &Server{
			exp:     exp,
			clients: make(map[string]*rpc.Client),
		}
		rpcsrv := rpc.NewServer()
		if err := rpcsrv.Register(srv); err != nil {
			log.Fatal("rpcsrv register failed: ", err)
		}
		rpcsrv.Accept(listener)
		listener.Close()
		return
	}

	imp, err := netchan.Import("tcp", addr)
	if err != nil {
		log.Fatal("netchan import failed: ", err)
	}
	srvconn, err := ncnet.Dial(imp, "ctl")
	if err != nil {
		log.Fatal("ncnet dial failed: ", err)
	}
	srv := rpc.NewClient(srvconn)

	var clientId string
	if err := srv.Call("Server.Publish", clientName, &clientId); err != nil {
		log.Fatal("publish failed: %v", err)
	}

	clientsrv := rpc.NewServer()
	if err := clientsrv.Register(Client{}); err != nil {
		log.Fatal("clientsrv register failed: ", err)
	}

	clientconn, err := ncnet.Dial(imp, clientId)
	if err != nil {
		log.Fatalf("ncnet dial %q failed: %v", clientId, err)
	}

	go clientsrv.ServeConn(clientconn)
	interact(srv)
	clientconn.Close()
	time.Sleep(0.1e9) // wait for close to propagate
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
