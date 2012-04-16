package seq

import (
	plan9 "code.google.com/p/rog-go/new9p"
	"errors"
)

type Req interface {
	Ttype() interface{}
}

type Result interface {
	Rtype() interface{}
}

type CompositeReq interface {
	Req
	Do(seq *Sequencer, f File) error // executes action. must result in one result.
}

type BasicReq interface {
	Req
	basic()
}

type File interface {
	Do(op BasicReq) (Result, error)
	IsOpen() bool
	IsDir() bool
	IsInSequence() bool
	FileSys() FileSys
}

type FileSys interface {
	NewFile() (File, error)
	StartSequence() (Sequence, <-chan Result, error)
}

type Sequence interface {
	Do(f File, op BasicReq) error
	FileSys() FileSys
	Error() error
}

var Eaborted = errors.New("sequence aborted")

type basicReq int

const (
	opClone basicReq = iota
	opCreate
	opWalk
	opOpen
	opRead
	opWrite
	opRemove
	opStat
	opWstat
	opClunk
	opAbort
	opNonseq
	opNone
)

type (
	CloneReq struct {
		F File
	}
	CloneResult struct{}

	CreateReq struct {
		Name string
		Perm plan9.Perm
		Mode uint8
	}
	CreateResult struct {
		Q plan9.Qid
	}

	WalkReq struct {
		Name string
	}
	WalkResult struct {
		Q plan9.Qid
	}

	OpenReq struct {
		Mode uint8
	}
	OpenResult struct {
		Q plan9.Qid
	}

	NonseqReq struct {
	}
	NonseqResult struct {
	}

	ReadReq struct {
		Data   []byte
		Offset int64
	}
	ReadResult struct {
		Count int
	}

	RemoveReq struct {
	}
	RemoveResult struct {
	}

	WriteReq struct {
		Data   []byte
		Offset int64
	}
	WriteResult struct {
		Count int
	}

	StatReq    struct{}
	StatResult struct {
		Stat plan9.Dir
	}

	WstatReq struct {
		Stat plan9.Dir
	}
	WstatResult struct{}

	ClunkReq    struct{}
	ClunkResult struct{}

	// AbortReq is always replied to with an error.
	AbortReq struct {
	}

	StringResult string
)

func (CloneReq) Ttype() interface{}  { return opClone }
func (CreateReq) Ttype() interface{} { return opCreate }
func (WalkReq) Ttype() interface{}   { return opWalk }
func (OpenReq) Ttype() interface{}   { return opOpen }
func (ReadReq) Ttype() interface{}   { return opRead }
func (WriteReq) Ttype() interface{}  { return opWrite }
func (StatReq) Ttype() interface{}   { return opStat }
func (WstatReq) Ttype() interface{}  { return opWstat }
func (RemoveReq) Ttype() interface{} { return opRemove }
func (ClunkReq) Ttype() interface{}  { return opClunk }
func (AbortReq) Ttype() interface{}  { return opAbort }
func (NonseqReq) Ttype() interface{} { return opNonseq }

func (CloneResult) Rtype() interface{}  { return opClone }
func (CreateResult) Rtype() interface{} { return opCreate }
func (WalkResult) Rtype() interface{}   { return opWalk }
func (OpenResult) Rtype() interface{}   { return opOpen }
func (ReadResult) Rtype() interface{}   { return opRead }
func (WriteResult) Rtype() interface{}  { return opWrite }
func (StatResult) Rtype() interface{}   { return opStat }
func (WstatResult) Rtype() interface{}  { return opWstat }
func (ClunkResult) Rtype() interface{}  { return opClunk }
func (RemoveResult) Rtype() interface{} { return opRemove }
func (NonseqResult) Rtype() interface{} { return opNonseq }
func (StringResult) Rtype() interface{} { return opNone }

func (CloneReq) basic()  {}
func (CreateReq) basic() {}
func (WalkReq) basic()   {}
func (OpenReq) basic()   {}
func (ReadReq) basic()   {}
func (WriteReq) basic()  {}
func (StatReq) basic()   {}
func (WstatReq) basic()  {}
func (ClunkReq) basic()  {}
func (RemoveReq) basic() {}
func (AbortReq) basic()  {}
func (NonseqReq) basic() {}
