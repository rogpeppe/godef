package client

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"sync"
	//"log"
	"bytes"
	plan9 "code.google.com/p/rog-go/new9p"
)

func getuser() string { return os.Getenv("USER") }

type Fid struct {
	c      *Conn
	qid    plan9.Qid
	fid    uint32
	mode   uint8
	flags  uint8 // fOpen | fAlloc
	seq    *seq9p
	offset int64
	f      sync.Mutex
}

const (
	fOpen = 1 << iota
	fAlloc
	fPending
)

func callers(n int) string {
	var b bytes.Buffer
	prev := false
	for {
		_, file, line, ok := runtime.Caller(n + 1)
		if !ok {
			return b.String()
		}
		if prev {
			fmt.Fprintf(&b, " ")
		}
		fmt.Fprintf(&b, "%s:%d", file, line)
		n++
		prev = true
	}
	return ""
}

func (fid *Fid) Close() (err error) {
	checkSeq(nil, fid)
	//log.Printf("Closed called from %s", callers(1))
	switch {
	case fid == nil:
		return nil
	case fid.flags&fPending != 0:
		panic("close of pending fid")
	case fid.flags&fAlloc != 0:
		tx := &plan9.Fcall{Type: plan9.Tclunk, Fid: fid.fid}
		_, err = fid.c.rpc(tx)
	}
	fid.c.putfid(fid)
	return err
}

func (fid *Fid) Create(name string, mode uint8, perm plan9.Perm) error {
	checkSeq(nil, fid)
	tx := &plan9.Fcall{Type: plan9.Tcreate, Fid: fid.fid, Name: name, Mode: mode, Perm: perm}
	rx, err := fid.c.rpc(tx)
	if err != nil {
		return err
	}
	fid.mode = mode
	fid.qid = rx.Qid
	return nil
}

func (fid *Fid) Dirread() ([]*plan9.Dir, error) {
	checkSeq(nil, fid)
	buf := make([]byte, plan9.STATMAX)
	n, err := fid.Read(buf)
	if err != nil {
		return nil, err
	}
	return plan9.UnmarshalDirs(buf[0:n])
}

func (fid *Fid) Dirreadall() ([]*plan9.Dir, error) {
	checkSeq(nil, fid)
	buf, err := ioutil.ReadAll(fid)
	if len(buf) == 0 {
		return nil, err
	}
	return plan9.UnmarshalDirs(buf)
}

func (fid *Fid) Open(mode uint8) error {
	checkSeq(nil, fid)
	tx := &plan9.Fcall{Type: plan9.Topen, Fid: fid.fid, Mode: mode}
	rx, err := fid.c.rpc(tx)
	if err != nil {
		return err
	}
	fid.qid = rx.Qid
	fid.mode = mode
	fid.flags |= fOpen
	return nil
}

func (fid *Fid) Qid() plan9.Qid {
	checkSeq(nil, fid)
	return fid.qid
}

func (fid *Fid) Read(b []byte) (n int, err error) {
	return fid.ReadAt(b, -1)
}

func (fid *Fid) ReadAt(b []byte, offset int64) (n int, err error) {
	checkSeq(nil, fid)
	msize := fid.c.msize - plan9.IOHDRSZ
	n = len(b)
	if uint32(n) > msize {
		n = int(msize)
	}
	o := offset
	if o == -1 {
		fid.f.Lock()
		o = fid.offset
		fid.f.Unlock()
	}
	tx := &plan9.Fcall{Type: plan9.Tread, Fid: fid.fid, Offset: uint64(o), Count: uint32(n)}
	rx, err := fid.c.rpc(tx)
	if err != nil {
		return 0, err
	}
	if len(rx.Data) == 0 {
		return 0, io.EOF
	}
	copy(b, rx.Data)
	if offset == -1 {
		fid.f.Lock()
		fid.offset += int64(len(rx.Data))
		fid.f.Unlock()
	}
	return len(rx.Data), nil
}

func (fid *Fid) ReadFull(b []byte) (n int, err error) {
	checkSeq(nil, fid)
	return io.ReadFull(fid, b)
}

func (fid *Fid) Remove() error {
	checkSeq(nil, fid)
	if fid.c == nil || fid.flags&fAlloc == 0 {
		return errors.New("no such fid")
	}
	tx := &plan9.Fcall{Type: plan9.Tremove, Fid: fid.fid}
	_, err := fid.c.rpc(tx)
	fid.flags &^= fAlloc
	fid.Close()
	return err
}

func (fid *Fid) Seek(n int64, whence int) (int64, error) {
	checkSeq(nil, fid)
	switch whence {
	case 0:
		fid.f.Lock()
		fid.offset = n
		fid.f.Unlock()

	case 1:
		fid.f.Lock()
		n += fid.offset
		if n < 0 {
			fid.f.Unlock()
			return 0, Error("negative offset")
		}
		fid.offset = n
		fid.f.Unlock()

	case 2:
		d, err := fid.Stat()
		if err != nil {
			return 0, err
		}
		n += int64(d.Length)
		if n < 0 {
			return 0, Error("negative offset")
		}
		fid.f.Lock()
		fid.offset = n
		fid.f.Unlock()

	default:
		return 0, Error("bad whence in seek")
	}

	return n, nil
}

func (fid *Fid) Stat() (*plan9.Dir, error) {
	checkSeq(nil, fid)
	tx := &plan9.Fcall{Type: plan9.Tstat, Fid: fid.fid}
	rx, err := fid.c.rpc(tx)
	if err != nil {
		return nil, err
	}
	return plan9.UnmarshalDir(rx.Stat)
}

func (fid *Fid) Clone(newfid *Fid) error {
	checkSeq(nil, fid)
	if newfid.flags&(fPending|fAlloc) != 0 {
		panic("fid in use")
	}
	newfid.flags |= fPending
	// TODO: check that newfid has not already been created.
	tx := &plan9.Fcall{Type: plan9.Twalk, Fid: fid.fid, Newfid: newfid.fid}
	_, err := fid.c.rpc(tx)
	newfid.flags &^= fPending
	if err == nil {
		newfid.flags |= fAlloc
	}
	return err
}

func (fid *Fid) Walk(elem ...string) (*Fid, error) {
	checkSeq(nil, fid)
	wfid, err := fid.c.getfid()
	if err != nil {
		return nil, err
	}

	for nwalk := 0; ; nwalk++ {
		n := len(elem)
		if n > plan9.MAXWELEM {
			n = plan9.MAXWELEM
		}
		tx := &plan9.Fcall{Type: plan9.Twalk, Newfid: wfid.fid, Wname: elem[0:n]}
		if nwalk == 0 {
			tx.Fid = fid.fid
		} else {
			tx.Fid = wfid.fid
		}
		rx, err := fid.c.rpc(tx)
		if err == nil && len(rx.Wqid) != n {
			err = Error("file '" + strings.Join(elem, "/") + "' not found")
		}
		if err != nil {
			wfid.Close()
			return nil, err
		}
		if n == 0 {
			wfid.qid = fid.qid
		} else {
			wfid.qid = rx.Wqid[n-1]
		}
		elem = elem[n:]
		if len(elem) == 0 {
			break
		}
	}
	return wfid, nil
}

func (fid *Fid) Write(b []byte) (n int, err error) {
	return fid.WriteAt(b, -1)
}

func (fid *Fid) WriteAt(b []byte, offset int64) (n int, err error) {
	checkSeq(nil, fid)
	//log.Printf("WriteAt %d\n", offset)
	//defer func(){log.Printf("-> %d, %v\n", n, err)}()
	//log.Printf("msize %d\n", fid.c.msize);
	msize := fid.c.msize - plan9.IOHDRSIZE
	tot := 0
	n = len(b)
	first := true
	for tot < n || first {
		want := n - tot
		if uint32(want) > msize {
			want = int(msize)
		}
		got, err := fid.writeAt(b[tot:tot+want], offset)
		tot += got
		if err != nil {
			return tot, err
		}
		if offset != -1 {
			offset += int64(got)
		}
		first = false
	}
	return tot, nil
}

func (fid *Fid) writeAt(b []byte, offset int64) (n int, err error) {
	o := offset
	if o == -1 {
		fid.f.Lock()
		o = fid.offset
		fid.f.Unlock()
	}
	tx := &plan9.Fcall{Type: plan9.Twrite, Fid: fid.fid, Offset: uint64(o), Data: b}
	rx, err := fid.c.rpc(tx)
	if err != nil {
		return 0, err
	}
	if offset == -1 && rx.Count > 0 {
		fid.f.Lock()
		fid.offset += int64(rx.Count)
		fid.f.Unlock()
	}
	return int(rx.Count), nil
}

func (fid *Fid) Wstat(d *plan9.Dir) error {
	checkSeq(nil, fid)
	b, err := d.Bytes()
	if err != nil {
		return err
	}
	tx := &plan9.Fcall{Type: plan9.Twstat, Fid: fid.fid, Stat: b}
	_, err = fid.c.rpc(tx)
	return err
}

func checkSeq(seq *seq9p, fid *Fid) {
	switch {
	case seq == nil && fid.seq != nil:
		//log.Printf("seq fid %d used outside sequence, flags %x, callers %s\n", fid.fid, fid.flags, callers(1))
		panic("seq fid used outside sequence")
	case seq != nil && fid.seq != nil && seq != fid.seq:
		//log.Printf("seq fid %d used in wrong sequence t%d, callers %s\n", fid.fid, seq.tag, callers(1))
		panic("seq fid used in wrong sequence")
	}
}
