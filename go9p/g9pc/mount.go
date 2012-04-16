// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package g9pc

import (
	"code.google.com/p/rog-go/go9p/g9p"
	"net"
)

// Creates an authentication fid for the specified user. Returns the fid, if
// successful, or an Error.
func (clnt *Client) Auth(user g9p.User, aname string) (*Fid, error) {
	fid := clnt.fidAlloc()
	tc := clnt.newFcall()
	err := g9p.PackTauth(tc, fid.Fid, user.Name(), aname, uint32(user.Id()), clnt.dotu)
	if err != nil {
		return nil, err
	}

	_, err = clnt.rpc(tc)
	if err != nil {
		return nil, err
	}

	fid.User = user
	return fid, nil
}

// Creates a fid for the specified user that points to the root
// of the file server's file tree. Returns a Fid pointing to the root,
// if successful, or an Error.
func (clnt *Client) Attach(afid *Fid, user g9p.User, aname string) (*Fid, error) {
	var afno uint32

	if afid != nil {
		afno = afid.Fid
	} else {
		afno = g9p.NOFID
	}

	fid := clnt.fidAlloc()
	tc := clnt.newFcall()
	err := g9p.PackTattach(tc, fid.Fid, afno, user.Name(), aname, uint32(user.Id()), clnt.dotu)
	if err != nil {
		return nil, err
	}

	rc, err := clnt.rpc(tc)
	if err != nil {
		return nil, err
	}
	if rc.Type == g9p.Rerror {
		return nil, &g9p.Error{rc.Error, int(rc.Errornum)}
	}

	fid.Qid = rc.Qid
	fid.User = user
	return fid, nil
}

// Connects to a file server and attaches to it as the specified user.
func Mount(netw, addr, aname string, user g9p.User, log g9p.Logger) (*Client, error) {
	conn, err := net.Dial(netw, addr)
	if conn == nil {
		return nil, err
	}
	clnt, err := NewClient(conn, 8192+g9p.IOHDRSZ, true, log)
	if clnt == nil {
		return nil, err
	}

	fid, err := clnt.Attach(nil, user, aname)
	if err != nil {
		clnt.Unmount()
		return nil, err
	}

	clnt.root = fid
	return clnt, nil
}

// Closes the connection to the file sever.
func (clnt *Client) Unmount() {
	clnt.conn.Close()
}
