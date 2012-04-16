// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package g9pc

import "code.google.com/p/rog-go/go9p/g9p"

// Reads count bytes starting from offset from the file associated with the fid.
// Returns a slice with the data read, if the operation was successful, or an
// Error.
func (fid *Fid) BRead(offset uint64, count int) ([]byte, error) {
	clnt := fid.Client
	tc := clnt.newFcall()
	err := g9p.PackTread(tc, fid.Fid, offset, uint32(count))
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

	return rc.Data, nil
}

func (fid *Fid) ReadAt(buf []byte, offset uint64) (int, error) {
	data, err := fid.BRead(offset, len(buf))
	if err != nil {
		return 0, err
	}
	return copy(buf, data), nil
}

// Reads up to len(buf) bytes from the File. Returns the number
// of bytes read, or an Error.
func (file *File) Read(buf []byte) (int, error) {
	n, err := file.ReadAt(buf, file.offset)
	if err == nil {
		file.offset += uint64(n)
	}

	return n, err
}

// Reads up to len(buf) bytes from the file starting from offset.
// Returns the number of bytes read, or an Error.
func (file *File) ReadAt(buf []byte, offset uint64) (int, error) {
	return file.fid.ReadAt(buf, offset)
}

// Reads exactly len(buf) bytes from the File starting from offset.
// Returns the number of bytes read (could be less than len(buf) if
// end-of-file is reached), or an Error.
func (file *File) Readn(buf []byte, offset uint64) (int, error) {
	ret := 0
	for len(buf) > 0 {
		n, err := file.ReadAt(buf, offset)
		if err != nil {
			return 0, err
		}

		if n == 0 {
			break
		}

		buf = buf[n:len(buf)]
		offset += uint64(n)
		ret += n
	}

	return ret, nil
}

// Reads the content of the directory associated with the File.
// Returns an array of maximum num entries (if num is 0, returns
// all entries from the directory). If the operation fails, returns
// an Error.
func (file *File) Readdir(num int) ([]*g9p.Dir, error) {
	buf := make([]byte, file.fid.Client.msize-g9p.IOHDRSZ)
	dirs := make([]*g9p.Dir, 32)
	pos := 0
	for {
		n, err := file.Read(buf)
		if err != nil {
			return nil, err
		}

		if n == 0 {
			break
		}

		for b := buf[0:n]; len(b) > 0; {
			var d *g9p.Dir
			d, err = g9p.UnpackDir(b, file.fid.Client.dotu)
			if err != nil {
				return nil, err
			}

			b = b[d.Size+2 : len(b)]
			if pos >= len(dirs) {
				s := make([]*g9p.Dir, len(dirs)+32)
				copy(s, dirs)
				dirs = s
			}

			dirs[pos] = d
			pos++
			if num != 0 && pos >= num {
				break
			}
		}
	}

	return dirs[0:pos], nil
}
