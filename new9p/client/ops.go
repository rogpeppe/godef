package client

import (
	"errors"
	"fmt"
	//"log"
	plan9 "code.google.com/p/rog-go/new9p"
	"code.google.com/p/rog-go/new9p/seq"
	"container/list"
)

func (fid *Fid) File() seq.File {
	return (*file9p)(fid)
}

type file9p Fid
type filesys9p Conn
type seq9p struct {
	c   *Conn
	tag uint16
	err error

	// access to the remaining fields guarded by c.w
	fids     map[uint32]*Fid
	replyEOF bool
	doEOF    bool
	q        queue
}

func (sq *seq9p) Error() error {
	return sq.err
}

func (fs *filesys9p) NewFile() (seq.File, error) {
	c := (*Conn)(fs)
	fid, err := c.getfid()
	if err != nil {
		return nil, err
	}
	return (*file9p)(fid), nil
}

func (fs *filesys9p) StartSequence() (seq.Sequence, <-chan seq.Result, error) {
	c := (*Conn)(fs)
	rxc := make(chan *plan9.Fcall, 1)
	tag, err := c.newtag(rxc)
	if err != nil {
		return nil, nil, err
	}
	resultc := make(chan seq.Result)
	sq := &seq9p{c: c, tag: tag}
	go sq.fcall2result(rxc, resultc)

	sq.c.w.Lock()
	defer c.w.Unlock()
	tx := &plan9.Fcall{Type: plan9.Tbegin, Tag: sq.tag}
	sq.q.Put(req{nil, nil, tx})
	if err := sq.c.write(tx); err != nil {
		return nil, nil, err
	}

	return sq, resultc, nil
}

type req struct {
	fid *Fid
	op  seq.Req
	tx  *plan9.Fcall
}

func (sq *seq9p) Do(f seq.File, op seq.BasicReq) error {
	var fid *Fid
	if f != nil {
		fid = (*Fid)(f.(*file9p))
		if fid.c != sq.c {
			//log.Printf("fs mismatch")
			return errors.New("mismatched filesys")
		}
		checkSeq(sq, fid)
		//log.Printf("seq9p.Do(seq %p, %p(%d, seq %p), %#v) (callers %s)", sq, fid, fid.fid, fid.seq, op, callers(1))
	} else {
		//log.Printf("seq9p.Do(nil, %#v) (callers %s)", op, callers(1))
	}
	sq.c.w.Lock()
	defer sq.c.w.Unlock()
	if sq.doEOF || (sq.replyEOF && op != nil) {
		//log.Printf("sequence has already terminated")
		return errors.New("sequence has terminated")
	}
	var tx *plan9.Fcall
	switch op := op.(type) {
	case nil:
		//log.Printf("seq9p sending terminate message")
		tx = &plan9.Fcall{Type: plan9.Tend}
	case seq.AbortReq:
		tx = &plan9.Fcall{Type: plan9.Tflush, Oldtag: sq.tag}
	case seq.CloneReq:
		if op.F == nil {
			panic("newfid is nil")
		}
		newfid := (*Fid)(op.F.(*file9p))
		if newfid.flags&(fAlloc|fPending) != 0 {
			panic("fid in use")
		}
		newfid.flags |= fPending
		newfid.seq = sq
		if sq.fids == nil {
			sq.fids = make(map[uint32]*Fid)
		}
		sq.fids[newfid.fid] = newfid
		tx = &plan9.Fcall{Type: plan9.Twalk, Newfid: newfid.fid}
	case seq.NonseqReq:
		tx = &plan9.Fcall{Type: plan9.Tnonseq}
	case seq.WalkReq:
		tx = &plan9.Fcall{Type: plan9.Twalk, Newfid: fid.fid, Wname: []string{op.Name}}
	case seq.CreateReq:
		tx = &plan9.Fcall{Type: plan9.Tcreate, Mode: op.Mode, Perm: op.Perm, Name: op.Name}
	case seq.RemoveReq:
		tx = &plan9.Fcall{Type: plan9.Tremove}
	case seq.OpenReq:
		// TODO sanity check mode
		tx = &plan9.Fcall{Type: plan9.Topen, Mode: op.Mode}
	case seq.ReadReq:
		// TODO range check offset
		tx = &plan9.Fcall{Type: plan9.Tread, Offset: uint64(op.Offset), Count: uint32(len(op.Data))}
	case seq.WriteReq:
		tx = &plan9.Fcall{Type: plan9.Twrite, Offset: uint64(op.Offset), Data: op.Data}
	case seq.StatReq:
		tx = &plan9.Fcall{Type: plan9.Tstat}
	case seq.WstatReq:
		b, err := op.Stat.Bytes()
		if err != nil {
			return err
		}
		tx = &plan9.Fcall{Type: plan9.Twstat, Stat: b}
	case seq.ClunkReq:
		tx = &plan9.Fcall{Type: plan9.Tclunk}
	default:
		panic("unknown request type")
	}
	if op != nil {
		tx.Fid = fid.fid
	}
	tx.Tag = sq.tag
	sq.q.Put(req{fid, op, tx})
	if err := sq.c.write(tx); err != nil {
		return err
	}
	if op == nil {
		sq.doEOF = true
		sq.putfids()
	}

	return nil
}

func (sq *seq9p) fcall2result(rxc <-chan *plan9.Fcall, resultc chan<- seq.Result) {
	defer func() {
		sq.replyEOF = true
		sq.putfids()
		close(resultc)
		sq.c.w.Unlock()
	}()
	for {
		rx := <-rxc
		sq.c.w.Lock()
		if rx == nil {
			sq.err = sq.c.getErr()
			return
		}
		rq := sq.q.Get()
		if rx.Type != rq.tx.Type+1 && rx.Type != plan9.Rerror {
			// TODO what do we do here
			panic(fmt.Sprintf("mismatched replies; sent %v; got %v", rq.tx, rx))
		}
		var result seq.Result
		switch rx.Type {
		case plan9.Rbegin:
		case plan9.Rend:
			return
		case plan9.Rerror:
			switch op := rq.op.(type) {
			case seq.CloneReq:
				newfid := (*Fid)(op.F.(*file9p))
				newfid.flags &^= fPending
				newfid.seq = nil
				sq.putfid(newfid)
				newfid.Close()
			}
			sq.err = errors.New(rx.Ename)
			return
		case plan9.Rwalk:
			switch len(rx.Wqid) {
			case 0:
				nfid := (*Fid)(rq.op.(seq.CloneReq).F.(*file9p))
				nfid.qid = rq.fid.qid
				nfid.mode = rq.fid.mode
				nfid.flags |= fAlloc
				nfid.flags &^= fPending
				nfid.offset = rq.fid.offset
				result = seq.CloneResult{}
			case 1:
				rq.fid.qid = rx.Wqid[0]
				result = seq.WalkResult{rx.Wqid[0]}
			default:
				panic("unexpected Rwalk qid count")
			}
		case plan9.Ropen:
			rq.fid.mode = rq.op.(seq.OpenReq).Mode
			rq.fid.flags |= fOpen
			rq.fid.qid = rx.Qid
			result = seq.OpenResult{rx.Qid}
		case plan9.Rnonseq:
			sq.putfid(rq.fid)
			result = seq.NonseqResult{}
		case plan9.Rcreate:
			rq.fid.mode = rq.op.(seq.CreateReq).Mode
			rq.fid.flags |= fOpen
			rq.fid.qid = rx.Qid
			result = seq.CreateResult{rx.Qid}
		case plan9.Rwrite:
			result = seq.WriteResult{int(rx.Count)} // TODO: check overflow?
		case plan9.Rread:
			data := rq.op.(seq.ReadReq).Data
			copy(data, rx.Data)
			result = seq.ReadResult{len(rx.Data)}
		case plan9.Rstat:
			d, err := plan9.UnmarshalDir(rx.Stat)
			if err != nil {
				// TODO try to recover here
				panic("bad dir structure from server")
			}
			result = seq.StatResult{*d}
		case plan9.Rwstat:
			result = seq.WstatResult{}
		case plan9.Rremove:
			rq.fid.flags &^= fAlloc
			sq.putfid(rq.fid)
			rq.fid.Close()
			result = seq.RemoveResult{}
		case plan9.Rclunk:
			rq.fid.flags &^= fAlloc
			sq.putfid(rq.fid)
			rq.fid.Close()
			result = seq.ClunkResult{}
		default:
			// TODO ensure sanity checked before it gets here.
			panic(fmt.Sprintf("unexpected rmessage: %v", rx))
		}
		//log.Printf("fcall2result %#v (rq %#v)\n", result, rq)
		sq.c.w.Unlock()
		if result != nil {
			resultc <- result
		}
	}

}

func (sq *seq9p) putfid(fid *Fid) {
	fid.seq = nil
	delete(sq.fids, fid.fid)
}

// putfids clunks all the fids if the sequence has terminated
// with an error. It is called with sq.c.w held.
// fids can be reused only after Tend has been
// sent and the final reply has been seen.
func (sq *seq9p) putfids() {
	//log.Printf("putfids; doEOF %v; replyEOF %v; fids %p; callers %s", sq.doEOF, sq.replyEOF, sq.fids, callers(1))
	if sq.doEOF && sq.replyEOF && sq.fids != nil {
		if sq.err != nil {
			for _, fid := range sq.fids {
				fid.flags &^= fAlloc | fPending
				fid.seq = nil
				fid.Close()
			}
		} else {
			for _, fid := range sq.fids {
				if fid.flags&fPending != 0 {
					panic("fid still pending")
				}
				fid.seq = nil
			}
		}
		sq.fids = nil
	}
}

func (sq *seq9p) FileSys() seq.FileSys {
	return (*filesys9p)(sq.c)
}

func (fid *file9p) IsDir() bool {
	if fid.flags&fPending != 0 {
		panic("fid is not allocated")
	}
	return (fid.qid.Type & plan9.QTDIR) != 0
}

func (fid *file9p) IsOpen() bool {
	if fid.flags&fPending != 0 {
		panic("fid is not allocated") // TODO ???
	}
	return (fid.flags & fOpen) != 0
}

func (fid *file9p) IsInSequence() bool {
	return fid.seq != nil
}

func (fid *file9p) FileSys() seq.FileSys {
	return (*filesys9p)(fid.c)
}

func (file *file9p) Do(op seq.BasicReq) (seq.Result, error) {
	fid := (*Fid)(file)
	switch op := op.(type) {
	case seq.CloneReq:
		newfid := (*Fid)(op.F.(*file9p))
		if newfid.flags&(fPending|fAlloc|fOpen) != 0 {
			panic("fid in use")
		}
		newfid.flags |= fPending
		tx := &plan9.Fcall{Type: plan9.Twalk, Fid: fid.fid, Newfid: newfid.fid}
		_, err := fid.c.rpc(tx)
		newfid.flags &^= fPending
		if err != nil {
			newfid.Close()
			return nil, err
		}
		newfid.flags = fid.flags
		newfid.qid = fid.qid
		return seq.CloneResult{}, nil

	case seq.WalkReq:
		tx := &plan9.Fcall{Type: plan9.Twalk, Fid: fid.fid, Newfid: fid.fid, Wname: []string{op.Name}}
		rx, err := fid.c.rpc(tx)
		if err != nil {
			return nil, err
		}
		fid.qid = rx.Qid
		return seq.WalkResult{fid.qid}, nil

	case seq.OpenReq:
		if err := fid.Open(op.Mode); err != nil {
			return nil, err
		}
		return seq.OpenResult{fid.qid}, nil

	case seq.ReadReq:
		n, err := fid.ReadAt(op.Data, op.Offset)
		if err != nil {
			return nil, err
		}
		return seq.ReadResult{n}, nil

	case seq.WriteReq:
		n, err := fid.WriteAt(op.Data, op.Offset)
		if err != nil {
			return nil, err
		}
		return seq.WriteResult{n}, nil

	case seq.StatReq:
		d, err := fid.Stat()
		if err != nil {
			return nil, err
		}
		return seq.StatResult{*d}, nil

	case seq.WstatReq:
		if err := fid.Wstat(&op.Stat); err != nil {
			return nil, err
		}
		return seq.WstatResult{}, nil

	case seq.ClunkReq:
		fid.Close()
		return seq.ClunkResult{}, nil

	case seq.CreateReq:
		tx := &plan9.Fcall{Type: plan9.Tcreate, Fid: fid.fid, Name: op.Name, Mode: op.Mode, Perm: op.Perm}
		rx, err := fid.c.rpc(tx)
		if err != nil {
			return nil, err
		}
		fid.qid = rx.Qid
		fid.mode = op.Mode
		return seq.CreateResult{fid.qid}, nil
	case seq.RemoveReq:
		if fid.flags&fAlloc == 0 {
			panic("remove of unalloced fid") // TODO better error
		}
		tx := &plan9.Fcall{Type: plan9.Tremove, Fid: fid.fid}
		_, err := fid.c.rpc(tx)
		fid.flags &^= fAlloc
		fid.Close()
		if err != nil {
			return nil, err
		}
		return seq.RemoveResult{}, err
	}
	panic(fmt.Sprintf("unknown request type %T", op))
}

type queue list.List

func (q *queue) Put(x req) {
	(*list.List)(q).PushBack(x)
}

func (q *queue) Get() req {
	l := (*list.List)(q)
	return l.Remove(l.Front()).(req)
}
