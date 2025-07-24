// +build !plan9

package client

import (
	"strings"

	"9fans.net/go/plan9"
)

type Fsys struct {
	root *Fid
}

func (c *Conn) Auth(uname, aname string) (*Fid, error) {
	conn, err := c.conn()
	if err != nil {
		return nil, err
	}
	afidnum, err := conn.newfidnum()
	if err != nil {
		return nil, err
	}
	tx := &plan9.Fcall{Type: plan9.Tauth, Afid: afidnum, Uname: uname, Aname: aname}
	rx, err := conn.rpc(tx, nil)
	if err != nil {
		conn.putfidnum(afidnum)
		return nil, err
	}
	return conn.newFid(afidnum, rx.Qid), nil
}

func (c *Conn) Attach(afid *Fid, user, aname string) (*Fsys, error) {
	conn, err := c.conn()
	if err != nil {
		return nil, err
	}
	fidnum, err := conn.newfidnum()
	if err != nil {
		return nil, err
	}
	tx := &plan9.Fcall{Type: plan9.Tattach, Afid: plan9.NOFID, Fid: fidnum, Uname: user, Aname: aname}
	if afid != nil {
		tx.Afid = afid.fid
	}
	rx, err := conn.rpc(tx, nil)
	if err != nil {
		conn.putfidnum(fidnum)
		return nil, err
	}
	return &Fsys{conn.newFid(fidnum, rx.Qid)}, nil
}

var accessOmode = [8]uint8{
	0,
	plan9.OEXEC,
	plan9.OWRITE,
	plan9.ORDWR,
	plan9.OREAD,
	plan9.OEXEC, // only approximate
	plan9.ORDWR,
	plan9.ORDWR, // only approximate
}

func (fs *Fsys) Access(name string, mode int) error {
	if mode == plan9.AEXIST {
		_, err := fs.Stat(name)
		return err
	}
	fid, err := fs.Open(name, accessOmode[mode&7])
	if fid != nil {
		fid.Close()
	}
	return err
}

func (fs *Fsys) Create(name string, mode uint8, perm plan9.Perm) (*Fid, error) {
	i := strings.LastIndex(name, "/")
	var dir, elem string
	if i < 0 {
		elem = name
	} else {
		dir, elem = name[0:i], name[i+1:]
	}
	fid, err := fs.root.Walk(dir)
	if err != nil {
		return nil, err
	}
	err = fid.Create(elem, mode, perm)
	if err != nil {
		fid.Close()
		return nil, err
	}
	return fid, nil
}

func (fs *Fsys) Open(name string, mode uint8) (*Fid, error) {
	fid, err := fs.root.Walk(name)
	if err != nil {
		return nil, err
	}
	if err := fid.Open(mode); err != nil {
		fid.Close()
		return nil, err
	}
	return fid, nil
}

func (fs *Fsys) Remove(name string) error {
	fid, err := fs.root.Walk(name)
	if err != nil {
		return err
	}
	return fid.Remove()
}

func (fs *Fsys) Stat(name string) (*plan9.Dir, error) {
	fid, err := fs.root.Walk(name)
	if err != nil {
		return nil, err
	}
	d, err := fid.Stat()
	fid.Close()
	return d, err
}

func (fs *Fsys) Wstat(name string, d *plan9.Dir) error {
	fid, err := fs.root.Walk(name)
	if err != nil {
		return err
	}
	err = fid.Wstat(d)
	fid.Close()
	return err
}

// Close closes the Fid underlying fs.
func (fs *Fsys) Close() error {
	return fs.root.Close()
}
