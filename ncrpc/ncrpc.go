// The ncrpc package provides an RPC interface layered onto
// a netchan connection.
package ncrpc

import (
	"os"
	"rog-go.googlecode.com/hg/ncnet"
	"netchan"
	"rpc"
)

// Server represents the server RPC interface.
// Instances should implement a set of methods
// that they wish to export from the Server, according
// to the rules in the rpc package.
// Init is called once to initialise the server with a given
// Exporter.
type Server interface {
	Init(*netchan.Exporter)
}

// NewServer creates a new RPC-over-netchan
// server. It exports the rpc interface provided
// by srv over a netchan connection to any clients
// and returns the netchan connection, which can
// then be exported over the network, for example
// with Listen.
// It reserves the use of netchan channels prefixed
// with "ncnet.ctl".
func NewServer(srv Server) (*netchan.Exporter, os.Error) {
	rpcsrv := rpc.NewServer()
	err := rpcsrv.Register(srv)
	if err != nil {
		return nil, err
	}
	exp := netchan.NewExporter()
	nclis, err := ncnet.Listen(exp, "ncnet.ctl")
	if err != nil {
		return nil, err
	}
	srv.Init(exp)
	go func() {
		rpcsrv.Accept(nclis)
		nclis.Close()
	}()
	return exp, nil
}

// Dial makes a connection to an ncrpc server
// on the given network address, imports the
// netchan connection and the rpc service,
// and returns them,
func Dial(network, addr string) (*netchan.Importer, *rpc.Client, os.Error) {
	imp, err := netchan.NewImporter("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	srvconn, err := ncnet.Dial(imp, "ncnet.ctl")
	if err != nil {
		return nil, nil, err
	}
	return imp, rpc.NewClient(srvconn), nil
}
