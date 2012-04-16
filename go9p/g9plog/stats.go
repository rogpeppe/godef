package g9plog

import (
	"code.google.com/p/rog-go/go9p/g9p"
	"container/list"
	"fmt"
	"net/http"
	"sync"
)

type httpStats struct {
	mu    sync.Mutex
	conns list.List
	maxId int
}

const (
	Packets = 1 << iota
)

type Logger struct {
	mu       sync.Mutex
	history  list.List
	maxHist  int
	isClient bool
	flags    int
	path     string
	name     string

	// traffic stats:
	nreqs    int
	tmsgsize int64
	rmsgsize int64
	npend    int
	maxpend  int
}

var stats httpStats

func init() {
	http.Handle("/go9p/", &stats)
}

var _ g9p.Logger = (*Logger)(nil)

func NewClient(name string, maxHist int, flags int) *Logger {
	stats.mu.Lock()
	id := stats.maxId
	stats.maxId++
	log := &Logger{name: name, flags: flags, path: fmt.Sprintf("/go9p/client/%d", id), isClient: true, maxHist: maxHist}
	stats.conns.PushFront(log)
	http.Handle(log.path, (*internalLogger)(log))
	stats.mu.Unlock()
	return log
}

func (log *Logger) Log9p(f *g9p.Fcall) {
	if f == nil {
		http.Handle(log.path, nil)
		stats.mu.Lock()
		for e := stats.conns.Front(); e != nil; e = e.Next() {
			if e.Value == log {
				stats.conns.Remove(e)
				break
			}
		}
		stats.mu.Unlock()
		return
	}
	log.mu.Lock()
	if f.IsTmsg() {
		log.npend++
		log.nreqs++
		if log.npend > log.maxpend {
			log.maxpend = log.npend
		}
		log.tmsgsize += int64(f.Size)
	} else {
		log.rmsgsize += int64(f.Size)
		// TODO: cater for flushes
		log.npend--
	}
	if log.maxHist != 0 && f.Type != 0 {
		nf := new(g9p.Fcall)
		*nf = *f
		if log.flags&Packets != 0 {
			nf.Pkt = make([]byte, len(f.Pkt))
			copy(nf.Pkt, f.Pkt)
		} else {
			nf.Pkt = nil
		}
		log.history.PushFront(nf)
		if log.history.Len() > log.maxHist && log.maxHist != -1 {
			log.history.Remove(log.history.Back())
		}
	}

	log.mu.Unlock()
}

func (stats *httpStats) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "<html><body>")
	defer fmt.Fprint(w, "</body></html>")

	stats.mu.Lock()
	defer stats.mu.Unlock()
	hasServers := false
	for e := stats.conns.Front(); e != nil; e = e.Next() {
		c := e.Value.(*Logger)
		if !c.isClient {
			if !hasServers {
				fmt.Fprintf(w, "<h1>Server connections</h1>")
				hasServers = true
			}
			fmt.Fprintf(w, "<a href='%s'>%s</a><br>", c.path, c.name)
		}
	}
	hasClients := false
	for e := stats.conns.Front(); e != nil; e = e.Next() {
		c := e.Value.(*Logger)
		if c.isClient {
			if !hasClients {
				fmt.Fprintf(w, "<h1>Client connections</h1>")
				hasServers = true
			}
			fmt.Fprintf(w, "<a href='%s'>%s</a><br>", c.path, c.name)
		}
	}
	if !hasServers && !hasClients {
		fmt.Fprintf(w, "<h1>No connections</h1>")
	}
}

// use this type to avoid exporting the ServeHTTP method on Logger.
type internalLogger Logger

func (log *internalLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctype := "Server"
	if log.isClient {
		ctype = "Client"
	}
	fmt.Fprintf(w, "<html><body><h1>%s connection %s</h1>", ctype, log.name)
	defer fmt.Fprint(w, "</body></html>")

	log.mu.Lock()
	defer log.mu.Unlock()
	fmt.Fprintf(w, "<p>Number of processed requests: %d", log.nreqs)
	fmt.Fprintf(w, "<br>Tmsg bytes: %d", log.tmsgsize)
	fmt.Fprintf(w, "<br>Rmsg bytes: %d", log.rmsgsize)
	fmt.Fprintf(w, "<br>Pending requests: %d; max %d", log.npend, log.maxpend)

	h := &log.history
	type tag struct {
		t   int
		r   int
		err bool
	}
	if h.Len() == 0 {
		fmt.Fprintf(w, "<h2>No 9P messages recorded</h2>")
		return
	}
	fmt.Fprintf(w, "<h2>Last %d 9P messages</h2>", h.Len())
	tags := make(map[uint16]*tag)
	ftags := make([]*tag, h.Len())
	i := 0
	// scan through history, matching tmsgs with rmsgs
	for e := h.Back(); e != nil; e, i = e.Prev(), i+1 {
		f := e.Value.(*g9p.Fcall)
		old, hasOld := tags[f.Tag]
		if f.IsTmsg() {
			ftags[i] = &tag{t: i, r: -1, err: old != nil}
			tags[f.Tag] = ftags[i]
		} else {
			if old != nil {
				old.r = i
				ftags[i] = old
				tags[f.Tag] = nil
			} else {
				// if we've previously seen an r-message, then the
				// tag map entry will exist, but be nil, so we know
				// there's an error
				ftags[i] = &tag{t: -1, r: i, err: hasOld}
			}
		}
	}
	tags = nil
	i = 0
	for e := h.Back(); e != nil; e, i = e.Prev(), i+1 {
		f := e.Value.(*g9p.Fcall)
		label := ""
		t := ftags[i]
		if f.IsTmsg() {
			if t.r != -1 {
				label = fmt.Sprintf("<a href='#fc%d'>&#10164;</a>", t.r)
			}
		} else {
			if t.t != -1 {
				label = fmt.Sprintf("<a href='#fc%d'>&#10166;</a>", t.t)
			}
		}
		// TODO change colour to red when t.err is true
		fmt.Fprintf(w, "<br id='fc%d'>%d: %s%s", i, i, f, label)
	}
}
