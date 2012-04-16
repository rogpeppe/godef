package client

import (
	plan9 "code.google.com/p/rog-go/new9p"
	"code.google.com/p/rog-go/new9p/seq"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Ns struct {
	Root *NsFile
	Dot  *NsFile
}

type NsFile struct {
	offset int64
	f      seq.File
}

type nsResultType bool
type OpResults []seq.Result

func (OpResults) Rtype() interface{} { return nsResultType(false) }

type PathWalkResult []plan9.Qid

func (PathWalkResult) Rtype() interface{} { return nsResultType(true) }

func (f *NsFile) IsDir() bool {
	return f.f.IsDir()
}

func (f *NsFile) IsOpen() bool {
	return f.f.IsOpen()
}

func (f *NsFile) File() seq.File {
	return f.f
}

func NewNsFile(f seq.File) *NsFile {
	if f == nil {
		return nil
	}
	return &NsFile{0, f}
}

func isSlash(c int) bool {
	return c == '/'
}

func Elements(name string) []string {
	// Split, delete dot.
	elem := strings.FieldsFunc(name, isSlash)
	j := 0
	for _, e := range elem {
		if e != "." {
			elem[j] = e
			j++
		}
	}
	return elem[0:j]
}

func (ns *Ns) path(name string) (*NsFile, []string) {
	if name == "" {
		return ns.Dot, nil
	}
	f := ns.Dot
	if name[0] == '/' {
		f = ns.Root
	}
	// Split, delete dot.
	elem := strings.FieldsFunc(name, isSlash)
	j := 0
	for _, e := range elem {
		if e != "." {
			elem[j] = e
			j++
		}
	}
	return f, elem[0:j]
}

func (ns *Ns) Walk(name string) (*NsFile, error) {
	sq, results := seq.NewSequencer()
	f, elem := ns.path(name)
	go func() {
		<-results
		_, ok := <-results
		if ok {
			panic("expected closed")
		}
	}()

	f = f.SeqWalk(sq, elem...)
	sq.Do(nil, nil)
	sq.Wait()
	err := sq.Wait()
	if err == nil {
		return f, nil
	}
	return nil, err
}

func (ns *Ns) SeqWalk(sq *seq.Sequencer, name string) *NsFile {
	f, elem := ns.path(name)
	return f.SeqWalk(sq, elem...)
}

func (ns *Ns) Open(name string, mode uint8) (f *NsFile, err error) {
	f, _, err = ns.ops(name, seq.OpenReq{mode})
	return
}

func (ns *Ns) ReadStream(name string, nreqs, iounit int) io.ReadCloser {
	sq, replies := seq.NewSequencer()
	go func() {
		<-replies // walk
		<-replies // open
		<-replies // stream
		sq.Do(nil, nil)
		_, ok := <-replies
		if ok {
			panic("expected closed")
		}
	}()

	f := ns.SeqWalk(sq, name)
	sq.Do(f.f, seq.OpenReq{plan9.OREAD})
	return f.SeqReadStream(sq, nreqs, iounit)
}

func (ns *Ns) SeqCreate(sq *seq.Sequencer, path string, mode uint8, perm plan9.Perm) *NsFile {
	f, elem := ns.path(path)
	if len(elem) == 0 {
		panic("no path elements") // TODO more sensible error handling
	}
	subseq, results := sq.Subsequencer("seqcreate")
	go func() {
		<-results // walk result
		<-results // create result
		_, ok := <-results
		if ok {
			panic("expected closed")
		}
		subseq.Result(nil, subseq.Error())
	}()
	elem, name := elem[0:len(elem)-1], elem[len(elem)-1]
	f = f.SeqWalk(subseq, elem...)
	f.seqops(subseq, seq.CreateReq{name, perm, mode})
	subseq.Do(nil, nil)
	return f
}

func (ns *Ns) Create(name string, mode uint8, perm plan9.Perm) (*NsFile, error) {
	sq, replies := seq.NewSequencer()
	go func() {
		<-replies
		_, ok := <-replies
		if ok {
			panic("expected closed")
		}
	}()

	f := ns.SeqCreate(sq, name, mode, perm)
	sq.Do(nil, nil)

	if err := sq.Wait(); err != nil {
		return nil, err
	}
	return f, nil
}

func (ns *Ns) SeqRemove(sq *seq.Sequencer, name string) {
	ns.seqops(sq, name, seq.RemoveReq{})
}

func (ns *Ns) Remove(name string) (err error) {
	_, _, err = ns.ops(name, seq.RemoveReq{})
	return
}

func (ns *Ns) Access(name string, mode uint8) (err error) {
	_, _, err = ns.ops(name, seq.RemoveReq{}, seq.ClunkReq{})
	return
}

func (ns *Ns) Stat(name string) (*plan9.Dir, error) {
	_, replies, err := ns.ops(name, seq.StatReq{}, seq.ClunkReq{})
	if err != nil {
		return nil, err
	}
	//log.Printf("stat replies: %#v\n", replies)
	d := replies[0].(seq.StatResult).Stat
	return &d, nil
}

func (ns *Ns) Wstat(name string, d *plan9.Dir) error {
	_, _, err := ns.ops(name, seq.WstatReq{*d}, seq.ClunkReq{})
	return err
}

//func (ns *Ns) Filesys() FileSys {
//	what to do about this?
//	to clone a file in a pipelined fashion, we have to create the
//	handle first.
//	except... if we use ns to do the walking, then it creates
//	the file for us, and it knows which fs to use.
//
//	so we don't need Ns.Filesys... i think!
//}

func (ns *Ns) Chdir(name string) error {
	f, err := ns.Walk(name)
	if err != nil {
		return err
	}
	if !f.IsDir() {
		f.Close()
		return errors.New("cannot chdir to non-directory")
	}
	ns.Dot.Close()
	ns.Dot = f
	return nil
}

func (ns *Ns) ops(name string, ops ...seq.Req) (*NsFile, []seq.Result, error) {
	sq, replies := seq.NewSequencer()
	c := make(chan []seq.Result)
	go func() {
		r, ok := <-replies
		if !ok {
			//log.Printf("ops got premature eof, seq %p, error %#v", seq, seq.Error())
			c <- nil
			return
		}
		_, ok = <-replies
		if ok {
			panic("expected closed")
		}
		c <- r.(OpResults)
	}()
	f := ns.seqops(sq, name, ops...)
	sq.Do(nil, nil)
	r := <-c
	if err := sq.Wait(); err != nil {
		return nil, r, err
	}
	//log.Printf("ops got eof, chan %p, seq %p, error %#v", replies, seq, seq.Error())
	return f, r, sq.Error()
}

func (ns *Ns) seqops(sq *seq.Sequencer, name string, ops ...seq.Req) *NsFile {
	subseq, results := sq.Subsequencer(fmt.Sprintf("ns.seqops(%#v)", ([]seq.Req)(ops)))
	go func() {
		<-results // walk result
		r := <-results
		_, ok := <-results
		if ok {
			panic("expected closed")
		}
		subseq.Result(r, subseq.Error())
	}()

	f, elem := ns.path(name)
	f = f.SeqWalk(subseq, elem...)
	f.seqops(subseq, ops...)
	subseq.Do(nil, nil)
	return f
}

func (f *NsFile) Clone() (*NsFile, error) {
	nf, err := f.f.FileSys().NewFile()
	if err != nil {
		return nil, err
	}
	_, err = f.f.Do(seq.CloneReq{nf})
	if err != nil {
		return nil, err
	}
	return NewNsFile(nf), nil
}

func (f *NsFile) Stat() (*plan9.Dir, error) {
	r, err := f.f.Do(seq.StatReq{})
	if err != nil {
		return nil, err
	}
	d := r.(seq.StatResult).Stat
	return &d, nil
}

func (f *NsFile) Wstat(dir *plan9.Dir) error {
	_, err := f.f.Do(seq.WstatReq{*dir})
	return err
}

func (f *NsFile) Remove() error {
	_, err := f.f.Do(seq.RemoveReq{})
	return err
}

func (f *NsFile) Open(mode uint8) error {
	if f.IsOpen() {
		return errors.New("file is already opened")
	}
	_, err := f.f.Do(seq.OpenReq{mode})
	return err
}

func (f *NsFile) ReadStream(nreqs, iounit int) io.ReadCloser {
	sq, replies := seq.NewSequencer()
	go func() {
		<-replies // ReadStream
		_, ok := <-replies
		if ok {
			panic("expected eof")
		}
	}()
	return f.SeqReadStream(sq, nreqs, iounit)
}

func (f *NsFile) Close() {
	f.f.Do(seq.ClunkReq{})
}

func (f *NsFile) Read(buf []byte) (int, error) {
	n, err := f.ReadAt(buf, f.offset)
	f.offset += int64(n) // TODO lock
	return n, err
}

func (f *NsFile) ReadAt(buf []byte, at int64) (int, error) {
	if !f.IsOpen() {
		return 0, errors.New("file is not opened")
	}
	r, err := f.f.Do(seq.ReadReq{buf, f.offset})
	if err != nil {
		return 0, err
	}
	n := r.(seq.ReadResult).Count
	if n == 0 {
		// TODO there's no unambiguous EOF indication in 9p,
		// so this is kinda incorrect.
		err = io.EOF
	}
	return n, err
}

func (f *NsFile) Write(data []byte) (n int, err error) {
	//log.Printf("%#v Write %d bytes", f, len(data))
	n, err = f.WriteAt(data, f.offset)
	f.offset += int64(n) // TODO lock
	return
}

func (f *NsFile) WriteAt(data []byte, offset int64) (int, error) {
	if !f.IsOpen() {
		return 0, errors.New("file is not opened")
	}
	r, err := f.f.Do(seq.WriteReq{data, f.offset})
	if err != nil {
		return 0, err
	}
	n := r.(seq.WriteResult).Count
	return n, err
}

func (f *NsFile) Dirread() ([]*plan9.Dir, error) {
	if !f.IsDir() {
		return nil, errors.New("not a directory")
	}
	buf := make([]byte, plan9.STATMAX)
	n, err := f.Read(buf)
	if err != nil {
		return nil, err
	}
	return plan9.UnmarshalDirs(buf[0:n])
}

func (f *NsFile) ops(ops ...seq.Req) ([]seq.Result, error) {
	c := make(chan OpResults)
	seq, replies := seq.NewSequencer()
	go func() {
		r, ok := <-replies
		if !ok {
			c <- nil
		}
		_, ok = <-replies
		if ok {
			panic("expected closed")
		}
		c <- r.(OpResults)
	}()
	f.seqops(seq, ops...)
	seq.Do(nil, nil)
	r := <-c
	return r, seq.Wait()
}

func (f *NsFile) seqops(sq *seq.Sequencer, ops ...seq.Req) {
	//log.Printf("file.seqops %#v", ([]seq.Req)(ops))
	subseq, results := sq.Subsequencer(fmt.Sprintf("nsfile.seqops(%#v)", ([]seq.Req)(ops)))
	go func() {
		result := make(OpResults, len(ops))
		i := 0
		for result[i] = range results {
			i++
		}
		//log.Printf("seqops [%#v] got eof, error %#v\n", ([]seq.Req)(ops), subseq.Error())
		// TODO(?): replies will be lost on error.
		subseq.Result(result, subseq.Error())
	}()
	for _, op := range ops {
		subseq.Do(f.f, op)
	}
	subseq.Do(nil, nil)
}

func (f *NsFile) Walk(elem ...string) (*NsFile, error) {
	seq, results := seq.NewSequencer()
	go func() {
		<-results // Walk
		_, ok := <-results
		if ok {
			panic("expected closed")
		}
	}()
	f = f.SeqWalk(seq, elem...)
	seq.Do(nil, nil)
	if err := seq.Wait(); err != nil {
		return nil, err
	}
	return f, nil
}

// result is PathWalkResult
func (f *NsFile) SeqWalk(sq *seq.Sequencer, path ...string) *NsFile {
	subseq, results := sq.Subsequencer(fmt.Sprintf("seqwalk(%#v)", ([]string)(path)))
	go func() {
		var qids PathWalkResult
		//log.Printf("seqwalk waiting on result chan %p", results)
		<-results
		for r := range results {
			qids = append(qids, r.(seq.WalkResult).Q)
		}
		//log.Printf("seqwalk got result eof")
		//log.Printf("seqwalk %p, %q, error %#v", subseq, subseq.name, subseq.Error())
		if err := subseq.Error(); err != nil {
			subseq.Result(nil, err)
			return
		}
		subseq.Result(qids, nil)
	}()
	nfile, err := f.f.FileSys().NewFile()
	if err != nil {
		panic("out of files")
	}
	//log.Printf("NewFile -> %#v\n", nfile)
	subseq.Do(f.f, seq.CloneReq{nfile})
	for _, name := range path {
		subseq.Do(nfile, seq.WalkReq{name})
	}
	subseq.Do(nil, nil)
	return NewNsFile(nfile)
}
