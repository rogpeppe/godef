package seq

import (
	"bytes"
	"fmt"
	"log"
	"runtime"
	"sync"
)

type mainSeq struct {
	mu         sync.Mutex
	newRequest chan<- seqRequest
	newSeq     <-chan Sequence
	newFs      chan<- FileSys
	currSeq    Sequence
	shutdown   bool
	done       chan error

	reentrantCheck chan bool
}

type Sequencer struct {
	error     error
	result    chan Result
	results   chan<- Result
	hasParent bool
	child     *Sequencer
	name      string
	main      *mainSeq
}

type seqRequest struct {
	subseq *Sequencer
	f      File
	op     BasicReq
}

type subseqCounter struct {
	npending int            // number of pending requests for this phase of a subsequence.
	seqs     *subseqStack   // subsequences for this phase.
	next     *subseqCounter // next phase in the queue.
}

type subseqStack struct {
	seq      *Sequencer   // subsequence.
	parent   *subseqStack // parent subsequence.
	npending int          // total pending requests for this subsequence.
	closed   bool         // Do(nil, nil) has been called for this sequence.
	final    bool         // part of the final error propagation stack.
}

type seqInfo struct {
	seq     Sequence
	results <-chan Result
}

type replier struct {
	newRequest   <-chan seqRequest
	newFs        <-chan FileSys
	newSeq       chan<- Sequence
	allSeqs      map[FileSys]seqInfo
	subseqs      *subseqStack
	seqhd        *subseqCounter
	seqtl        *subseqCounter
	totalPending int
	currSeq      seqInfo
}

// NewSequencer returns a new object that represents a stream
// of requests. The caller should arrange to receive the result
// of each request in turn on result. If one request fails,
// then all subsequent requests will fail - if this happens, the
// result channel will be closed early, and the error will be available
// from seq.Error().
// Within a given Sequence, calls to Do and Subsequence 
func NewSequencer() (*Sequencer, <-chan Result) {
	//log.Printf("NewSequencer, callers %s, %s, %s", caller(1), caller(2), caller(3))
	newFs := make(chan FileSys)
	newRequest := make(chan seqRequest)
	newSeq := make(chan Sequence)
	results := make(chan Result)

	seq := &Sequencer{
		name:    "root",
		result:  make(chan Result, 1),
		results: results,
		main: &mainSeq{
			newRequest:     newRequest,
			newFs:          newFs,
			newSeq:         newSeq,
			reentrantCheck: make(chan bool, 1),
			done:           make(chan error, 1),
		},
	}

	go seq.replier(newFs, newSeq, newRequest)
	//log.Printf("new root req %p", seq)
	return seq, results
}

func (m *mainSeq) enter() {
	select {
	case m.reentrantCheck <- true:
	default:
		panic("reentrancy")
	}
}

func (m *mainSeq) leave() {
	<-m.reentrantCheck
}

// Subsequencer returns a Sequencer that is nested within parent.
// No requests on parent will take place until all requests on seq
// have been sent (this allows a subsequence to send requests
// in a separate goroutine without needing explicit synchronisation
// to ensure sequentiality).
// It behaves just as a Sequencer, except that after result has
// been closed, seq.Result must be called to provide the
// sub-sequencer's result.
func (parent *Sequencer) Subsequencer(name string) (seq *Sequencer, result <-chan Result) {
	results := make(chan Result)

	seq = &Sequencer{
		name:      name,
		result:    make(chan Result, 1),
		results:   results,
		main:      parent.main,
		hasParent: true,
	}
	parent.child = seq
	//log.Printf("do %p: new subseq %p (%s)", parent, seq, seq.name)
	seq.main.newRequest <- seqRequest{seq, nil, nil}
	return seq, results
}

// Error returns any error that has occurred when executing
// the sequence. The returned value is only valid when the
// Sequencer's result channel has been closed.
func (seq *Sequencer) Error() error {
	return seq.error
}

// Wait blocks until the root Sequencer has terminated.
func (seq *Sequencer) Wait() (err error) {
	err = <-seq.main.done
	seq.main.done <- err
	return
}

// When a Sequencer's result channel has been closed, Result
// must be called to provide the results of the sequence to
// its parent. If err is nil, val will be sent on the parent's
// result channel; otherwise the parent's result channel will be
// closed and err returned from its Error().
// It is an error for err to be non nil if the sequence
// has not terminated with an error.
func (seq *Sequencer) Result(val Result, err error) {
	//log.Printf("seq %p, name %s: result (on chan %p) -> %#v, %#v", seq, seq.name, seq.result, val, err)
	if err == nil {
		seq.result <- val
	} else {
		if seq.error == nil {
			panic("error result with no sequence error")
		}
		seq.error = err
		close(seq.result)
	}
}

func (seqs *subseqStack) String() string {
	if seqs == nil {
		return "[]"
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "[%p[%d]", seqs.seq, seqs.npending)
	for s := seqs.parent; s != nil; s = s.parent {
		fmt.Fprintf(&b, "->%p[%d]", s.seq, s.npending)
	}
	fmt.Fprintf(&b, "]")
	return b.String()
}

func (c *subseqCounter) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "[")
	for ; c != nil; c = c.next {
		fmt.Fprintf(&b, "[")
		fmt.Fprintf(&b, "%d ", c.npending)
		fmt.Fprintf(&b, "%v", c.seqs)
		fmt.Fprintf(&b, "] ")
	}
	fmt.Fprintf(&b, "]")
	return b.String()
}

// Do adds the given request to the queue of requests
// on seq. It returns an error if it could not do so.
func (seq *Sequencer) Do(f File, anyReq Req) (err error) {
	main := seq.main
	if main == nil {
		return
	}
	main.enter()
	defer main.leave()
	if anyReq == nil {
		if f == nil {
			if seq.hasParent {
				main.newRequest <- seqRequest{}
				seq.hasParent = false
			} else {
				close(main.newRequest)
			}
		}
		return
	}
	var op BasicReq
	switch anyReq := anyReq.(type) {
	case BasicReq:
		op = anyReq
	case CompositeReq:
		return anyReq.Do(seq, f)
	default:
		panic("unknown request type")
	}
	// The check for f!=nil is necessary because AbortReq
	// doesn't apply to a particular file. We send it to
	// the current sequence, which will abort, causing the
	// whole thing to abort. If there is no current sequence,
	// then we've nothing to abort, so we ignore the request.
	if f != nil {
		pfs := f.FileSys()
		if main.currSeq == nil || pfs != main.currSeq.FileSys() {
			main.newFs <- pfs
			main.currSeq = <-main.newSeq
		}
	}
	if main.currSeq == nil {
		// This can happen if an AbortReq is sent when
		// no other requests have been sent, or when
		// a sequence has yielded an error
		// and <-main.newSeq has returned nil.
		// In both cases, we can ignore the request.
		return
	}

	// lock is necessary so that we do not send any requests
	// after replier has terminated sequences
	main.newRequest <- seqRequest{nil, f, op}
	main.mu.Lock()
	if main.shutdown {
		close(main.newRequest)
		seq.main = nil
	} else {
		err = main.currSeq.Do(f, op) // TODO: check error here.
	}
	main.mu.Unlock()
	return err
}

func (seq *Sequencer) replier(newFs <-chan FileSys, newSeq chan<- Sequence, newRequest <-chan seqRequest) {
	r := &replier{
		allSeqs:    make(map[FileSys]seqInfo),
		subseqs:    &subseqStack{seq: seq},
		newRequest: newRequest,
		newFs:      newFs,
		newSeq:     newSeq,
	}
	r.seqhd = &subseqCounter{seqs: r.subseqs}
	r.seqtl = r.seqhd

	error := r.run()
	if error != nil {
		r.propagateError(error)
		r.discardRequests()
		seq.closeSequences(r.currSeq, r.allSeqs, false)
		if seq.error == nil {
			panic("expected error")
		}
		seq.main.done <- seq.error
		//log.Printf("after error propagation, seqhd: %v\n", r.seqhd)
		return
	}

	//log.Printf("replier: ok termination, stack: %v", r.subseqs)
	if r.seqhd.next != nil {
		//log.Printf("replier: termination with requests still pending, npend %d, next npend %d, totalPending %d", r.seqhd.npending, r.seqhd.next.npending, r.totalPending)
		panic("termination with requests still pending")

	}
	if r.subseqs != nil {
		panic("termination with non empty stack")
	}
	close(seq.results)
	seq.closeSequences(r.currSeq, r.allSeqs, true)
	seq.main.done <- nil
	return
}

func (r *replier) propagateError(error error) {
	// Mark stack at head of queue as final,
	for seqs := r.seqhd.seqs; seqs != nil; seqs = seqs.parent {
		seqs.final = true
	}
	// Close all sequences that are not part of final stack.
	for c := r.seqhd; c != nil; c = c.next {
		seqs := c.seqs
		if seqs.seq != nil && !seqs.final {
			seqs.seq.error = Eaborted
			log.Printf("replier: closing seq %p", seqs.seq)
			close(seqs.seq.results)
			seqs.seq = nil
		}
	}
	// Propagate error from each final subsequence
	// to its parent, making sure
	// that each one is closed before sending the result,
	// to avoid overlap of Dos.
	log.Printf("replier: final error propagation, seqhd: %v", r.seqhd)
	for seqs := r.seqhd.seqs; seqs != nil; seqs = seqs.parent {
		seq := seqs.seq
		seq.error = error
		log.Printf("replier: closing seq %p", seq)
		close(seq.results)

		// wait for seq to be closed completely
		// before providing the result to the parent.
		// This means that a parent will not get a result
		// before its subsequence has properly closed.
		for !seqs.closed {
			select {
			case p, ok := <-r.newRequest:
				r.request(p, !ok)
			case <-r.newFs:
				r.newSeq <- nil
			}
		}
		parent := seqs.parent
		if parent != nil {
			log.Printf("replier: waiting for result from %p", seq)
			v, ok := <-seq.result
			if !ok {
				// error has changed
				parent.seq.error = seq.error
				log.Printf("replier: %p error result: %v", seq, seq.error)
			} else {
				// parent will get the result value, then Eaborted
				parent.seq.error = Eaborted
				log.Printf("replier(error): parent %p(%q) results <- %#v", seqs.parent.seq, seqs.parent.seq.name, v)
				parent.seq.results <- v
			}
		}
	}
}

func (r *replier) discardRequests() {
	// discard any subsequent requests.
	for r.newRequest != nil {
		select {
		case p, ok := <-r.newRequest:
			if !ok {
				r.newRequest = nil
			} else if p.subseq != nil {
				p.subseq.error = Eaborted
				//log.Printf("close(%p) (discard)", p.subseq.results)
				close(p.subseq.results)
			}

		case <-r.newFs:
			r.newSeq <- nil
		}
	}
}

func (r *replier) run() error {
	// invariant: r.seqhd != r.seqtl => r.seqhd.npending > 0
	for {
		//log.Printf("replier: loop %v", r.seqhd)
		newFs := r.newFs
		// Don't let Do give us a new Sequence until all the replies
		// from the old Sequence have arrived.
		if r.totalPending > 0 {
			newFs = nil
		} else if r.newRequest == nil {
			break
		}

		select {
		case fs := <-newFs:
			//log.Printf("replier: new sequence")
			r.newSeq <- r.newSequence(fs)
		case p, ok := <-r.newRequest:
			//log.Printf("replier: got request %#v", p)
			r.request(p, !ok)
		case p, ok := <-r.currSeq.results:
			if !ok {
				// result could only have been closed as a result
				// of an error, as none of the underlying sequences have
				// been closed yet.
				error := r.currSeq.seq.Error()
				//log.Printf("replier: result closed, error %#v", error)
				if error == nil {
					panic("expected error when sequence ended prematurely")
				}
				return error
			}
			//log.Printf("replier: result %#v; %#v -> seq %p (phase %d, seq %d, total %d)\n", p, r, r.seqhd.seqs.seq, r.seqhd.npending, r.seqhd.seqs.npending,  r.totalPending)
			//log.Printf("replier: sending result to %p, name %s\n", r.seqhd.seqs.seq.results, r.seqhd.seqs.seq.name)
			r.seqhd.seqs.seq.results <- p
			r.totalPending--
			r.seqhd.npending--
			r.seqhd.seqs.npending--
		}
		// maintain invariant
		r.closeSubsequences()
	}
	return nil
}

func (r *replier) request(p seqRequest, closed bool) {
	if closed {
		if r.subseqs.parent != nil {
			panic("close with outstanding subsequences")
		}
		r.seqtl.seqs.closed = true
		r.subseqs = nil
		r.newRequest = nil
		return
	}

	switch {
	case p.op != nil:
		//log.Printf("replier: op %#v, seq %p (phase %d, seq %d, total %d)\n", p.op, r.seqtl.seqs.seq, r.seqtl.npending, r.seqtl.seqs.npending,  r.totalPending)
		r.totalPending++
		r.seqtl.npending++
		r.seqtl.seqs.npending++
	case p.subseq == nil:
		//log.Printf("replier: subseq %p closed", r.seqtl.seqs.seq)
		// subsequence has been closed: pop the subsequence
		// stack, and add a new counter to the queue.
		r.seqtl.seqs.closed = true
		r.subseqs = r.subseqs.parent
		r.seqtl.next = &subseqCounter{seqs: r.subseqs}
		r.seqtl = r.seqtl.next
	default:
		//log.Printf("replier: subseq %p(%q), parent %p", p.subseq, p.subseq.name, r.subseqs.seq)
		// New subsequence. Push it onto the stack
		// and add a new counter to the queue.
		// If the previous counter was redundant, we replace it.
		// This means that when replies arrive,
		// a counter with npending==0 unambiguously
		// implies that the sequence has terminated.
		r.subseqs = &subseqStack{seq: p.subseq, parent: r.subseqs}
		if r.seqtl.npending == 0 && !r.seqtl.seqs.closed {
			//log.Printf("replier: replacing phase")
			r.seqtl.seqs = r.subseqs
		} else {
			//log.Printf("replier: adding phase")
			r.seqtl.next = &subseqCounter{seqs: r.subseqs}
			r.seqtl = r.seqtl.next
		}
	}
}

func (r *replier) newSequence(fs FileSys) Sequence {
	if r.currSeq = r.allSeqs[fs]; r.currSeq.seq == nil {
		s, results, err := fs.StartSequence()
		if err != nil {
			// TODO
		}
		r.currSeq = seqInfo{s, results}
		r.allSeqs[fs] = r.currSeq
	}
	return r.currSeq.seq
}

func (seq *Sequencer) closeSequences(currSeq seqInfo, allSeqs map[FileSys]seqInfo, ok bool) {
	seq.main.mu.Lock()
	seq.main.shutdown = true

	// terminate all sequences. we don't need to
	// abort the current sequence, as that must be
	// the one responsible for the error, so there is no need.
	for _, s := range allSeqs {
		if !ok && s.seq != currSeq.seq {
			// XXX do we need to be waiting on results here?
			s.seq.Do(nil, AbortReq{})
		}
		//log.Printf("doer: terminating sequence %p", s.seq)
		s.seq.Do(nil, nil)
	}
	for _, s := range allSeqs {
		_, ok := <-s.results
		if ok {
			panic("expected closed")
		}
	}
	seq.main.mu.Unlock()
}

// closeSubsequences closes subsequences we're now done with,
// waiting for their result values and passing them to their parents.
func (r *replier) closeSubsequences() {

	//log.Printf("replier: closeSubsequences %v", r.seqhd)
	for r.seqhd.npending == 0 && r.seqhd.next != nil {
		hd := r.seqhd.seqs
		r.seqhd = r.seqhd.next
		if !hd.closed {
			//log.Printf("replier: seq %p npending 0, but not closed", hd.seq)
			// just discard redundant unclosed sequences.
			continue
		}
		//log.Printf("replier: closeSubsequences: closing %p, chan %p", hd.seq, hd.seq.results)
		if hd.parent == nil {
			panic("hmm?")
		}
		close(hd.seq.results)
		// propagate the subsequence's result to its parent.
		v, ok := <-hd.seq.result
		if !ok {
			panic("unexpected error result, cannot happen!")
		}
		//log.Printf("closeSubsequences: sending propagated value %#v from %p(%q) to %p(%q)", v, hd.seq, hd.seq.name, hd.parent.seq, hd.parent.seq.name)
		hd.parent.seq.results <- v
	}
}

func caller(n int) string {
	_, file, line, ok := runtime.Caller(n + 1)
	if !ok {
		return "no-caller"
	}
	return fmt.Sprintf("%s:%d", file, line)
}
