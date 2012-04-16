// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package g9pc

import "code.google.com/p/rog-go/go9p/g9p"

// Clunks a fid. Returns nil if successful.
func (clnt *Client) Clunk(fid *Fid) (err error) {
	if fid.walked {
		tc := clnt.newFcall()
		err = g9p.PackTclunk(tc, fid.Fid)
		if err != nil {
			return
		}

		_, err = clnt.rpc(tc)
	}

	clnt.fidpool.putId(fid.Fid)
	fid.walked = false
	fid.Fid = g9p.NOFID
	return
}

// Closes a file. Returns nil if successful.
func (file *File) Close() error {
	// Should we cancel all pending requests for the File
	return file.fid.Client.Clunk(file.fid)
}
