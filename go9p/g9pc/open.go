// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package g9pc

import (
	"code.google.com/p/rog-go/go9p/g9p"
	"strings"
)

// Opens the file associated with the fid. Returns nil if
// the operation is successful.
func (clnt *Client) Open(fid *Fid, mode uint8) error {
	tc := clnt.newFcall()
	err := g9p.PackTopen(tc, fid.Fid, mode)
	if err != nil {
		return err
	}

	rc, err := clnt.rpc(tc)
	if err != nil {
		return err
	}
	if rc.Type == g9p.Rerror {
		return &g9p.Error{rc.Error, int(rc.Errornum)}
	}

	fid.Qid = rc.Qid
	fid.Iounit = rc.Iounit
	if fid.Iounit == 0 || fid.Iounit > clnt.msize-g9p.IOHDRSZ {
		fid.Iounit = clnt.msize - g9p.IOHDRSZ
	}
	fid.Mode = mode
	return nil
}

// Creates a file in the directory associated with the fid. Returns nil
// if the operation is successful.
func (clnt *Client) Create(fid *Fid, name string, perm uint32, mode uint8, ext string) error {
	tc := clnt.newFcall()
	err := g9p.PackTcreate(tc, fid.Fid, name, perm, mode, ext, clnt.dotu)
	if err != nil {
		return err
	}

	rc, err := clnt.rpc(tc)
	if err != nil {
		return err
	}
	if rc.Type == g9p.Rerror {
		return &g9p.Error{rc.Error, int(rc.Errornum)}
	}

	fid.Qid = rc.Qid
	fid.Iounit = rc.Iounit
	if fid.Iounit == 0 || fid.Iounit > clnt.msize-g9p.IOHDRSZ {
		fid.Iounit = clnt.msize - g9p.IOHDRSZ
	}
	fid.Mode = mode
	return nil
}

// Creates and opens a named file.
// Returns the file if the operation is successful, or an Error.
func (clnt *Client) FCreate(path string, perm uint32, mode uint8) (*File, error) {
	n := strings.LastIndex(path, "/")
	if n < 0 {
		n = 0
	}

	fid, err := clnt.FWalk(path[0:n])
	if err != nil {
		return nil, err
	}

	if path[n] == '/' {
		n++
	}

	err = clnt.Create(fid, path[n:], perm, mode, "")
	if err != nil {
		clnt.Clunk(fid)
		return nil, err
	}

	return &File{fid, 0}, nil
}

// Opens a named file. Returns the opened file, or an Error.
func (clnt *Client) FOpen(path string, mode uint8) (*File, error) {
	fid, err := clnt.FWalk(path)
	if err != nil {
		return nil, err
	}

	err = clnt.Open(fid, mode)
	if err != nil {
		clnt.Clunk(fid)
		return nil, err
	}

	return &File{fid, 0}, nil
}
