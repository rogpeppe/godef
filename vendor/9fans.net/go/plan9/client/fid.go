package client

import (
	"io"

	"9fans.net/go/plan9"
)

func (fid *Fid) Dirread() ([]*plan9.Dir, error) {
	buf := make([]byte, plan9.STATMAX)
	n, err := fid.Read(buf)
	if err != nil {
		return nil, err
	}
	return dirUnpack(buf[0:n])
}

func (fid *Fid) Dirreadall() ([]*plan9.Dir, error) {
	// Note: Cannot use ioutil.ReadAll / io.ReadAll here
	// because it assumes it can read small amounts.
	// Plan 9 requires providing a buffer big enough for
	// at least a single directory entry.
	var dirs []*plan9.Dir
	for {
		d, err := fid.Dirread()
		dirs = append(dirs, d...)
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return dirs, err
		}
	}
}

func dirUnpack(b []byte) ([]*plan9.Dir, error) {
	var err error
	dirs := make([]*plan9.Dir, 0, 10)
	for len(b) > 0 {
		if len(b) < 2 {
			err = io.ErrUnexpectedEOF
			break
		}
		n := int(b[0]) | int(b[1])<<8
		if len(b) < n+2 {
			err = io.ErrUnexpectedEOF
			break
		}
		var d *plan9.Dir
		d, err = plan9.UnmarshalDir(b[0 : n+2])
		if err != nil {
			break
		}
		b = b[n+2:]
		if len(dirs) >= cap(dirs) {
			ndirs := make([]*plan9.Dir, len(dirs), 2*cap(dirs))
			copy(ndirs, dirs)
			dirs = ndirs
		}
		n = len(dirs)
		dirs = dirs[0 : n+1]
		dirs[n] = d
	}
	return dirs, err
}

func (fid *Fid) ReadFull(b []byte) (n int, err error) {
	return io.ReadFull(fid, b)
}
