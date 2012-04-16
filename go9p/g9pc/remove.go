// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package g9pc

import "code.google.com/p/rog-go/go9p/g9p"

// Removes the file associated with the Fid. Returns nil if the
// operation is successful.
func (clnt *Client) Remove(fid *Fid) error {
	tc := clnt.newFcall()
	err := g9p.PackTremove(tc, fid.Fid)
	if err != nil {
		return err
	}

	rc, err := clnt.rpc(tc)
	clnt.fidpool.putId(fid.Fid)
	fid.Fid = g9p.NOFID

	if rc.Type == g9p.Rerror {
		return &g9p.Error{rc.Error, int(rc.Errornum)}
	}

	return err
}

// Removes the named file. Returns nil if the operation is successful.
func (clnt *Client) FRemove(path string) error {
	var err error
	fid, err := clnt.FWalk(path)
	if err != nil {
		return err
	}

	err = clnt.Remove(fid)
	return err
}
