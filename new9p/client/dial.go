package client

import (
	"net"
	"os"
)

func Dial(network, addr string) (*Conn, os.Error) {
	c, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return NewConn(c)
}

func DialService(service string) (*Conn, os.Error) {
	ns := os.Getenv("NAMESPACE")
	if ns == "" {
		return nil, Error("unknown name space")
	}
	return Dial("unix", ns+"/"+service)
}

func Mount(network, addr string, aname string) (*Fsys, os.Error) {
	c, err := Dial(network, addr)
	if err != nil {
		return nil, err
	}
	fsys, err := c.Attach(nil, getuser(), aname)
	if err != nil {
		c.Close()
	}
	return fsys, err
}

func MountService(service string) (*Fsys, os.Error) {
	c, err := DialService(service)
	if err != nil {
		return nil, err
	}
	fsys, err := c.Attach(nil, getuser(), "")
	if err != nil {
		c.Close()
	}
	return fsys, err
}
