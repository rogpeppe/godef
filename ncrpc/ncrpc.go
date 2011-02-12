// The ncrpc package provides an RPC interface layered onto
// a netchan connection.
package ncrpc

import (
	"os"
	"fmt"
	"io"
	"log"
	"net"
	"rog-go.googlecode.com/hg/ncnet"
	"netchan"
	"rpc"
	"sync"
)

// ServerRPC represents the server-side RPC server.
// Instances should implement a set of methods
// that they wish to export from the server, according
// to the rules in the rpc package.
// Init is called once with new Server instance.
type ServerRPC interface {
	Init(*Server)
}

type Server struct {
	Exporter      *netchan.Exporter
	RPCServer *rpc.Server
	mu       sync.Mutex
	clients  map[string]*rpc.Client
	clientid int
}

// Publish is an RPC method that allows a client to publish
// its own RPC interface. It is only public because it
// needs to be in order to be seen by rpc.Register.
// It is called (remotely) by Client.Serve.
func (srv *Server) Publish(name *string, clientId *string) os.Error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.clients[*name] != nil {
		return os.ErrorString("client name already exists")
	}
	*clientId = fmt.Sprintf("client%d", srv.clientid)
	srv.clientid++
	listener, err := ncnet.Listen(srv.Exporter, *clientId)
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
		err = client.Call("ClientRPC.Ping", &struct{}{}, &struct{}{})
		if err != nil {
			log.Printf("error on init: %v", err)
			return
		}
		srv.mu.Lock()
		srv.clients[*name] = client
		srv.mu.Unlock()
		// when call completes, client has left.
		client.Call("ClientRPC.Wait", &struct{}{}, &struct{}{})
		srv.mu.Lock()
		srv.clients[*name] = nil, false
		srv.mu.Unlock()
	}()
	return nil
}

// Client gets the RPC connection for a given client.
func (srv *Server) Client(name string) (c *rpc.Client) {
	srv.mu.Lock()
	c = srv.clients[name]
	srv.mu.Unlock()
	return
}

// ClientNames returns the list of all clients that have
// published RPC connections to the server.
func (srv *Server) ClientNames() (a []string) {
	srv.mu.Lock()
	for name := range srv.clients {
		a = append(a, name)
	}
	srv.mu.Unlock()
	return
}

// NewServer creates a new RPC-over-netchan
// server. It returns a new Server instance containing
// a netchan.Exporter and an rpc.Server which
// is listening on a channel within it.
// It reserves the use of netchan channels prefixed
// with "ncnet.ctl".
//
// Conventionally Register is called on the rpc.Server
// to export some server RPC methods, and Accept is
// then called on the netchan.Export to listen on the network.
func NewServer() (*Server, os.Error) {
	rpcsrv := rpc.NewServer()
	exp := netchan.NewExporter()
	nclis, err := ncnet.Listen(exp, "ncnet.ctl")
	if err != nil {
		return nil, err
	}
	srv := &Server{
		clients: make(map[string]*rpc.Client),
		Exporter: exp,
		RPCServer: rpcsrv,
	}
	go func() {
		rpcsrv.Accept(nclis)
		nclis.Close()
	}()
	return srv, nil
}

// Client represents an ncrpc client.
type Client struct {
	Importer *netchan.Importer
	Server *rpc.Client
}

// Import makes a connection to an ncrpc server
// and calls NewClient on it.
func Import(network, addr string) (*Client, os.Error) {
	conn, err := net.Dial(network, "", addr)
	if err != nil {
		return nil, err
	}
	return NewClient(conn)
}

// NewClient makes a the netchan connection from
// the given connection, imports the rpc service
// from that, and returns both in a new Client instance.
// It assumes that the server has been started
// with Server.
func NewClient(conn io.ReadWriter) (*Client, os.Error) {
	imp := netchan.NewImporter(conn)
	srvconn, err := ncnet.Dial(imp, "ncnet.ctl")
	if err != nil {
		return nil, err
	}
	return &Client{imp, rpc.NewClient(srvconn)}, nil
}

// Serve announces an RPC service on the client using the
// given name (which must currently be unique amongst all
// clients).
func (c *Client) Serve(clientName string, rpcServer *rpc.Server) os.Error {
	var clientId string
	rpcServer.RegisterName("ClientRPC", clientRPC{})		// TODO better name
	if err := c.Server.Call("Server.Publish", &clientName, &clientId); err != nil {
		return err
	}
	return nil
}

// clientRPC implements the methods that Server.Publish expects of a client.
type clientRPC struct {}

func (clientRPC) Ping(*struct{}, *struct{}) os.Error {
	return nil
}

// Wait blocks until the client is ready to leave.
// Currently that's forever.
func (clientRPC) Wait(*struct{}, *struct{}) os.Error {
	select {}
	return nil
}
