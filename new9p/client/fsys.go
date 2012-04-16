package client

import (
	"errors"

	plan9 "code.google.com/p/rog-go/new9p"
)

type Fsys struct {
	Root *Fid
}

func (c *Conn) Auth(uname, aname string) (*Fid, error) {
	afid, err := c.getfid()
	if err != nil {
		return nil, err
	}
	afid.flags |= fPending
	tx := &plan9.Fcall{Type: plan9.Tauth, Afid: afid.fid, Uname: uname, Aname: aname}
	rx, err := c.rpc(tx)
	afid.flags &^= fPending
	if err != nil {
		afid.Close()
		return nil, err
	}
	afid.flags |= fAlloc
	afid.qid = rx.Qid
	return afid, nil
}

func (c *Conn) Attach(afid *Fid, user, aname string) (*Fsys, error) {
	fid, err := c.getfid()
	if err != nil {
		return nil, err
	}
	tx := &plan9.Fcall{Type: plan9.Tattach, Afid: plan9.NOFID, Fid: fid.fid, Uname: user, Aname: aname}
	if afid != nil {
		tx.Afid = afid.fid
	}
	rx, err := c.rpc(tx)
	if err != nil {
		fid.Close()
		return nil, err
	}
	fid.qid = rx.Qid
	fid.flags |= fAlloc
	return &Fsys{fid}, nil
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
	path := Elements(name)
	n := len(path)
	if n == 0 {
		return nil, errors.New("create: empty path")
	}
	path, elem := path[0:n-1], path[n-1]

	fid, err := fs.Root.Walk(path...)
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

func (fs *Fsys) Walk(name string) (*Fid, error) {
	return fs.Root.Walk(Elements(name)...)
}

func (fs *Fsys) Open(name string, mode uint8) (*Fid, error) {
	fid, err := fs.Walk(name)
	if err != nil {
		return nil, err
	}
	err = fid.Open(mode)
	if err != nil {
		fid.Close()
		return nil, err
	}
	return fid, nil
}

func (fs *Fsys) Remove(name string) error {
	fid, err := fs.Walk(name)
	if err != nil {
		return err
	}
	return fid.Remove()
}

func (fs *Fsys) Stat(name string) (*plan9.Dir, error) {
	fid, err := fs.Walk(name)
	if err != nil {
		return nil, err
	}
	d, err := fid.Stat()
	fid.Close()
	return d, err
}

func (fs *Fsys) Wstat(name string, d *plan9.Dir) error {
	fid, err := fs.Walk(name)
	if err != nil {
		return err
	}
	err = fid.Wstat(d)
	fid.Close()
	return err
}
