// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package g9pc

import "code.google.com/p/rog-go/go9p/g9p"

// Returns the metadata for the file associated with the Fid, or an Error.
func (clnt *Client) Stat(fid *Fid) (*g9p.Dir, error) {
	tc := clnt.newFcall()
	err := g9p.PackTstat(tc, fid.Fid)
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

	return &rc.Dir, nil
}

// Returns the metadata for a named file, or an Error.
func (clnt *Client) FStat(path string) (*g9p.Dir, error) {
	fid, err := clnt.FWalk(path)
	if err != nil {
		return nil, err
	}

	d, err := clnt.Stat(fid)
	clnt.Clunk(fid)
	return d, err
}

// Modifies the data of the file associated with the Fid, or an Error.
func (clnt *Client) Wstat(fid *Fid, dir *g9p.Dir) error {
	tc := clnt.newFcall()
	err := g9p.PackTwstat(tc, fid.Fid, dir, clnt.dotu)
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

	return nil
}
