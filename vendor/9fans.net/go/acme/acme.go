// Package acme is a simple interface for interacting with acme windows.
//
// Many of the functions in this package take a format string and optional
// parameters.  In the documentation, the notation format, ... denotes the result
// of formatting the string and arguments using fmt.Sprintf.
package acme // import "9fans.net/go/acme"

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"9fans.net/go/draw"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

// A Win represents a single acme window and its control files.
type Win struct {
	id         int
	ctl        *client.Fid
	tag        *client.Fid
	body       *client.Fid
	addr       *client.Fid
	event      *client.Fid
	data       *client.Fid
	xdata      *client.Fid
	errors     *client.Fid
	ebuf       *bufio.Reader
	c          chan *Event
	next, prev *Win
	buf        []byte
	e2, e3, e4 Event
	name       string

	errorPrefix string
}

var windowsMu sync.Mutex
var windows, last *Win
var autoExit bool

var fsys *client.Fsys
var fsysErr error
var fsysOnce sync.Once

// AutoExit sets whether to call os.Exit the next time the last managed acme window is deleted.
// If there are no acme windows at the time of the call, the exit does not happen until one
// is created and then deleted.
func AutoExit(exit bool) {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	autoExit = exit
}

// New creates a new window.
func New() (*Win, error) {
	fsysOnce.Do(mountAcme)
	if fsysErr != nil {
		return nil, fsysErr
	}
	fid, err := fsys.Open("new/ctl", plan9.ORDWR)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 100)
	n, err := fid.Read(buf)
	if err != nil {
		fid.Close()
		return nil, err
	}
	a := strings.Fields(string(buf[0:n]))
	if len(a) == 0 {
		fid.Close()
		return nil, errors.New("short read from acme/new/ctl")
	}
	id, err := strconv.Atoi(a[0])
	if err != nil {
		fid.Close()
		return nil, errors.New("invalid window id in acme/new/ctl: " + a[0])
	}
	return Open(id, fid)
}

type WinInfo struct {
	ID int
	// TagLen holds the length of the tag in runes.
	TagLen int
	// TagLen holds the length of the body in runes.
	BodyLen    int
	IsDir      bool
	IsModified bool
	// Name and Tag are only populated when the
	// WinInfo has been obtained by calling the Windows
	// function, as they're not available by reading the ctl file.

	// Name holds the filename of the window.
	Name string

	// Tag holds the rest of the tag after the filename.
	Tag string

	// The Size and History fields can only be non-nil
	// when WinInfo has been obtained by calling
	// the Win.Info method, because that information
	// isn't available as part of the index file.
	Size *WinSizeInfo
}

type WinSizeInfo struct {
	Width    int
	Font     string
	TabWidth int
}

// A LogReader provides read access to the acme log file.
type LogReader struct {
	f   *client.Fid
	buf [8192]byte
}

func (r *LogReader) Close() error {
	return r.f.Close()
}

// A LogEvent is a single event in the acme log file.
type LogEvent struct {
	ID   int
	Op   string
	Name string
}

// Read reads an event from the acme log file.
func (r *LogReader) Read() (LogEvent, error) {
	n, err := r.f.Read(r.buf[:])
	if err != nil {
		return LogEvent{}, err
	}
	f := strings.SplitN(string(r.buf[:n]), " ", 3)
	if len(f) != 3 {
		return LogEvent{}, fmt.Errorf("malformed log event")
	}
	id, _ := strconv.Atoi(f[0])
	op := f[1]
	name := f[2]
	name = strings.TrimSpace(name)
	return LogEvent{id, op, name}, nil
}

// Log returns a reader reading the acme/log file.
func Log() (*LogReader, error) {
	fsysOnce.Do(mountAcme)
	if fsysErr != nil {
		return nil, fsysErr
	}
	f, err := fsys.Open("log", plan9.OREAD)
	if err != nil {
		return nil, err
	}
	return &LogReader{f: f}, nil
}

// Windows returns a list of the existing acme windows.
func Windows() ([]WinInfo, error) {
	fsysOnce.Do(mountAcme)
	if fsysErr != nil {
		return nil, fsysErr
	}
	index, err := fsys.Open("index", plan9.OREAD)
	if err != nil {
		return nil, err
	}
	defer index.Close()
	data, err := ioutil.ReadAll(index)
	if err != nil {
		return nil, err
	}
	var infos []WinInfo
	for _, line := range strings.Split(string(data), "\n") {
		if len(line) == 0 {
			continue
		}
		var info WinInfo
		tag, err := splitFields(line,
			&info.ID,
			&info.TagLen,
			&info.BodyLen,
			&info.IsDir,
			&info.IsModified,
		)
		if err != nil {
			return nil, fmt.Errorf("invalid index line %q: %v", line, err)
		}
		i := strings.Index(tag, " Del Snarf ")
		if i == -1 {
			return nil, fmt.Errorf("cannot determine filename in tag %q", tag)
		}
		info.Name = tag[:i]
		info.Tag = tag[i:]
		infos = append(infos, info)
	}
	return infos, nil
}

// Show looks and causes acme to show the window with the given name,
// returning that window.
// If this process has not created a window with the given name
// (or if any such window has since been deleted),
// Show returns nil.
func Show(name string) *Win {
	windowsMu.Lock()
	defer windowsMu.Unlock()

	for w := windows; w != nil; w = w.next {
		if w.name == name {
			if err := w.Ctl("show"); err != nil {
				w.dropLocked()
				return nil
			}
			return w
		}
	}
	return nil
}

// Open connects to the existing window with the given id.
// If ctl is non-nil, Open uses it as the window's control file
// and takes ownership of it.
func Open(id int, ctl *client.Fid) (*Win, error) {
	fsysOnce.Do(mountAcme)
	if fsysErr != nil {
		return nil, fsysErr
	}
	if ctl == nil {
		var err error
		ctl, err = fsys.Open(fmt.Sprintf("%d/ctl", id), plan9.ORDWR)
		if err != nil {
			return nil, err
		}
	}

	w := new(Win)
	w.id = id
	w.ctl = ctl
	w.next = nil
	w.prev = last
	if last != nil {
		last.next = w
	} else {
		windows = w
	}
	last = w
	return w, nil
}

// Addr writes format, ... to the window's addr file.
func (w *Win) Addr(format string, args ...interface{}) error {
	return w.Fprintf("addr", format, args...)
}

// CloseFiles closes all the open files associated with the window w.
// (These file descriptors are cached across calls to Ctl, etc.)
func (w *Win) CloseFiles() {
	w.ctl.Close()
	w.ctl = nil

	w.body.Close()
	w.body = nil

	w.addr.Close()
	w.addr = nil

	w.tag.Close()
	w.tag = nil

	w.event.Close()
	w.event = nil
	w.ebuf = nil

	w.data.Close()
	w.data = nil

	w.xdata.Close()
	w.xdata = nil

	w.errors.Close()
	w.errors = nil
}

// Ctl writes the command format, ... to the window's ctl file.
func (w *Win) Ctl(format string, args ...interface{}) error {
	return w.Fprintf("ctl", format+"\n", args...)
}

// Winctl deletes the window, writing `del' (or, if sure is true, `delete') to the ctl file.
func (w *Win) Del(sure bool) error {
	cmd := "del"
	if sure {
		cmd = "delete"
	}
	return w.Ctl(cmd)
}

// DeleteAll deletes all windows.
func DeleteAll() {
	for w := windows; w != nil; w = w.next {
		w.Ctl("delete")
	}
}

func (w *Win) OpenEvent() error {
	_, err := w.fid("event")
	return err
}

func (w *Win) fid(name string) (*client.Fid, error) {
	var f **client.Fid
	var mode uint8 = plan9.ORDWR
	switch name {
	case "addr":
		f = &w.addr
	case "body":
		f = &w.body
	case "ctl":
		f = &w.ctl
	case "data":
		f = &w.data
	case "event":
		f = &w.event
	case "tag":
		f = &w.tag
	case "xdata":
		f = &w.xdata
	case "errors":
		f = &w.errors
		mode = plan9.OWRITE
	default:
		return nil, errors.New("unknown acme file: " + name)
	}
	if *f == nil {
		var err error
		*f, err = fsys.Open(fmt.Sprintf("%d/%s", w.id, name), mode)
		if err != nil {
			return nil, err
		}
	}
	return *f, nil
}

// ReadAll
func (w *Win) ReadAll(file string) ([]byte, error) {
	f, err := w.fid(file)
	if err != nil {
		return nil, err
	}
	f.Seek(0, 0)
	return ioutil.ReadAll(f)
}

func (w *Win) ID() int {
	return w.id
}

func (w *Win) Name(format string, args ...interface{}) error {
	name := fmt.Sprintf(format, args...)
	if err := w.Ctl("name %s", name); err != nil {
		return err
	}
	w.name = name
	return nil
}

func (w *Win) Fprintf(file, format string, args ...interface{}) error {
	f, err := w.fid(file)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, format, args...)
	_, err = f.Write(buf.Bytes())
	return err
}

func (w *Win) Read(file string, b []byte) (n int, err error) {
	f, err := w.fid(file)
	if err != nil {
		return 0, err
	}
	return f.Read(b)
}

func (w *Win) ReadAddr() (q0, q1 int, err error) {
	f, err := w.fid("addr")
	if err != nil {
		return 0, 0, err
	}
	buf := make([]byte, 40)
	n, err := f.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return 0, 0, err
	}
	a := strings.Fields(string(buf[0:n]))
	if len(a) < 2 {
		return 0, 0, errors.New("short read from acme addr")
	}
	q0, err0 := strconv.Atoi(a[0])
	q1, err1 := strconv.Atoi(a[1])
	if err0 != nil || err1 != nil {
		return 0, 0, errors.New("invalid read from acme addr")
	}
	return q0, q1, nil
}

func (w *Win) Info() (WinInfo, error) {
	f, err := w.fid("ctl")
	if err != nil {
		return WinInfo{}, err
	}
	buf := make([]byte, 8192)
	n, err := f.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return WinInfo{}, err
	}
	line := string(buf[:n])
	info := WinInfo{
		Size: new(WinSizeInfo),
	}
	if _, err := splitFields(line,
		&info.ID,
		&info.TagLen,
		&info.BodyLen,
		&info.IsDir,
		&info.IsModified,
		&info.Size.Width,
		&info.Size.Font,
		&info.Size.TabWidth,
	); err != nil {
		return WinInfo{}, fmt.Errorf("invalid ctl contents %q: %v", line, err)
	}
	return info, nil
}

func (w *Win) Seek(file string, offset int64, whence int) (int64, error) {
	f, err := w.fid(file)
	if err != nil {
		return 0, err
	}
	return f.Seek(offset, whence)
}

func (w *Win) Write(file string, b []byte) (n int, err error) {
	f, err := w.fid(file)
	if err != nil {
		return 0, err
	}
	return f.Write(b)
}

const eventSize = 256

// An Event represents an event originating in a particular window.
// The fields correspond to the fields in acme's event messages.
// See http://swtch.com/plan9port/man/man4/acme.html for details.
type Event struct {
	// The two event characters, indicating origin and type of action
	C1, C2 rune

	// The character addresses of the action.
	// If the original event had an empty selection (OrigQ0=OrigQ1)
	// and was accompanied by an expansion (the 2 bit is set in Flag),
	// then Q0 and Q1 will indicate the expansion rather than the
	// original event.
	Q0, Q1 int

	// The Q0 and Q1 of the original event, even if it was expanded.
	// If there was no expansion, OrigQ0=Q0 and OrigQ1=Q1.
	OrigQ0, OrigQ1 int

	// The flag bits.
	Flag int

	// The number of bytes in the optional text.
	Nb int

	// The number of characters (UTF-8 sequences) in the optional text.
	Nr int

	// The optional text itself, encoded in UTF-8.
	Text []byte

	// The chorded argument, if present (the 8 bit is set in the flag).
	Arg []byte

	// The chorded location, if present (the 8 bit is set in the flag).
	Loc []byte
}

// ReadEvent reads the next event from the window's event file.
func (w *Win) ReadEvent() (e *Event, err error) {
	defer func() {
		if v := recover(); v != nil {
			e = nil
			err = errors.New("malformed acme event: " + v.(string))
		}
	}()

	if _, err = w.fid("event"); err != nil {
		return nil, err
	}

	e = new(Event)
	w.gete(e)
	e.OrigQ0 = e.Q0
	e.OrigQ1 = e.Q1

	// expansion
	if e.Flag&2 != 0 {
		e2 := new(Event)
		w.gete(e2)
		if e.Q0 == e.Q1 {
			e2.OrigQ0 = e.Q0
			e2.OrigQ1 = e.Q1
			e2.Flag = e.Flag
			e = e2
		}
	}

	// chorded argument
	if e.Flag&8 != 0 {
		e3 := new(Event)
		e4 := new(Event)
		w.gete(e3)
		w.gete(e4)
		e.Arg = e3.Text
		e.Loc = e4.Text
	}

	return e, nil
}

func (w *Win) gete(e *Event) {
	if w.ebuf == nil {
		w.ebuf = bufio.NewReader(w.event)
	}
	e.C1 = w.getec()
	e.C2 = w.getec()
	e.Q0 = w.geten()
	e.Q1 = w.geten()
	e.Flag = w.geten()
	e.Nr = w.geten()
	if e.Nr > eventSize {
		panic("event string too long")
	}
	r := make([]rune, e.Nr)
	for i := 0; i < e.Nr; i++ {
		r[i] = w.getec()
	}
	e.Text = []byte(string(r))
	if w.getec() != '\n' {
		panic("phase error")
	}
}

func (w *Win) getec() rune {
	c, _, err := w.ebuf.ReadRune()
	if err != nil {
		panic(err.Error())
	}
	return c
}

func (w *Win) geten() int {
	var (
		c rune
		n int
	)
	for {
		c = w.getec()
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c) - '0'
	}
	if c != ' ' {
		panic("event number syntax")
	}
	return n
}

// WriteEvent writes an event back to the window's event file,
// indicating to acme that the event should be handled internally.
func (w *Win) WriteEvent(e *Event) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%c%c%d %d \n", e.C1, e.C2, e.OrigQ0, e.OrigQ1)
	_, err := w.Write("event", buf.Bytes())
	return err
}

// EventChan returns a channel on which events can be read.
// The first call to EventChan allocates a channel and starts a
// new goroutine that loops calling ReadEvent and sending
// the result into the channel.  Subsequent calls return the
// same channel.  Clients should not call ReadEvent after calling
// EventChan.
func (w *Win) EventChan() <-chan *Event {
	if w.c == nil {
		w.c = make(chan *Event, 0)
		go w.eventReader()
	}
	return w.c
}

func (w *Win) eventReader() {
	for {
		e, err := w.ReadEvent()
		if err != nil {
			break
		}
		w.c <- e
	}
	w.c <- new(Event) // make sure event reader is done processing last event; drop might exit
	w.drop()
	close(w.c)
}

func (w *Win) drop() {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	w.dropLocked()
}

func (w *Win) dropLocked() {
	if w.prev == nil && w.next == nil && windows != w {
		return
	}
	if w.prev != nil {
		w.prev.next = w.next
	} else {
		windows = w.next
	}
	if w.next != nil {
		w.next.prev = w.prev
	} else {
		last = w.prev
	}
	w.prev = nil
	w.next = nil
	if autoExit && windows == nil {
		os.Exit(0)
	}
}

var fontCache struct {
	sync.Mutex
	m map[string]*draw.Font
}

// Font returns the window's current tab width (in zeros) and font.
func (w *Win) Font() (tab int, font *draw.Font, err error) {
	ctl := make([]byte, 1000)
	w.Seek("ctl", 0, 0)
	n, err := w.Read("ctl", ctl)
	if err != nil {
		return 0, nil, err
	}
	f := strings.Fields(string(ctl[:n]))
	if len(f) < 8 {
		return 0, nil, fmt.Errorf("malformed ctl file")
	}
	tab, _ = strconv.Atoi(f[7])
	if tab == 0 {
		return 0, nil, fmt.Errorf("malformed ctl file")
	}
	name := f[6]

	fontCache.Lock()
	font = fontCache.m[name]
	fontCache.Unlock()

	if font != nil {
		return tab, font, nil
	}

	var disp *draw.Display = nil
	font, err = disp.OpenFont(name)
	if err != nil {
		return tab, nil, err
	}

	fontCache.Lock()
	if fontCache.m == nil {
		fontCache.m = make(map[string]*draw.Font)
	}
	if fontCache.m[name] != nil {
		font = fontCache.m[name]
	} else {
		fontCache.m[name] = font
	}
	fontCache.Unlock()

	return tab, font, nil
}

// Blink starts the window tag blinking and returns a function that stops it.
// When stop returns, the blinking is over and the window state is clean.
func (w *Win) Blink() (stop func()) {
	c := make(chan struct{})
	go func() {
		t := time.NewTicker(1000 * time.Millisecond)
		defer t.Stop()
		dirty := false
		for {
			select {
			case <-t.C:
				dirty = !dirty
				if dirty {
					w.Ctl("dirty")
				} else {
					w.Ctl("clean")
				}
			case <-c:
				w.Ctl("clean")
				c <- struct{}{}
				return
			}
		}
	}()
	return func() {
		c <- struct{}{}
		<-c
	}
}

// Sort sorts the lines in the current address range
// according to the comparison function.
func (w *Win) Sort(less func(x, y string) bool) error {
	q0, q1, err := w.ReadAddr()
	if err != nil {
		return err
	}
	data, err := w.ReadAll("xdata")
	if err != nil {
		return err
	}
	suffix := ""
	lines := strings.Split(string(data), "\n")
	if lines[len(lines)-1] == "" {
		suffix = "\n"
		lines = lines[:len(lines)-1]
	}
	sort.SliceStable(lines, func(i, j int) bool { return less(lines[i], lines[j]) })
	w.Addr("#%d,#%d", q0, q1)
	w.Write("data", []byte(strings.Join(lines, "\n")+suffix))
	return nil
}

// PrintTabbed prints tab-separated columnated text to body,
// replacing single tabs with runs of tabs as needed to align columns.
func (w *Win) PrintTabbed(text string) {
	tab, font, _ := w.Font()

	lines := strings.SplitAfter(text, "\n")
	var allRows [][]string
	for _, line := range lines {
		if line == "" {
			continue
		}
		line = strings.TrimSuffix(line, "\n")
		allRows = append(allRows, strings.Split(line, "\t"))
	}

	var buf bytes.Buffer
	for len(allRows) > 0 {
		if row := allRows[0]; len(row) <= 1 {
			if len(row) > 0 {
				buf.WriteString(row[0])
			}
			buf.WriteString("\n")
			allRows = allRows[1:]
			continue
		}

		i := 0
		for i < len(allRows) && len(allRows[i]) > 1 {
			i++
		}

		rows := allRows[:i]
		allRows = allRows[i:]

		var wid []int
		if font != nil {
			for _, row := range rows {
				for len(wid) < len(row) {
					wid = append(wid, 0)
				}
				for i, col := range row {
					n := font.StringWidth(col)
					if wid[i] < n {
						wid[i] = n
					}
				}
			}
		}

		for _, row := range rows {
			for i, col := range row {
				buf.WriteString(col)
				if i == len(row)-1 {
					break
				}
				if font == nil || tab == 0 {
					buf.WriteString("\t")
					continue
				}
				pos := font.StringWidth(col)
				for pos <= wid[i] {
					buf.WriteString("\t")
					pos += tab - pos%tab
				}
			}
			buf.WriteString("\n")
		}
	}

	w.Write("body", buf.Bytes())
}

// Clear clears the window body.
func (w *Win) Clear() {
	w.Addr(",")
	w.Write("data", nil)
}

type EventHandler interface {
	Execute(cmd string) bool
	Look(arg string) bool
}

func (w *Win) loadText(e *Event, h EventHandler) {
	if len(e.Text) == 0 && e.Q0 < e.Q1 {
		w.Addr("#%d,#%d", e.Q0, e.Q1)
		data, err := w.ReadAll("xdata")
		if err != nil {
			w.Err(err.Error())
		}
		e.Text = data
	}
}

func (w *Win) EventLoop(h EventHandler) {
	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X': // execute
			cmd := strings.TrimSpace(string(e.Text))
			if !w.execute(h, cmd) {
				w.WriteEvent(e)
			}
		case 'l', 'L': // look
			// TODO(rsc): Expand selection, especially for URLs.
			w.loadText(e, h)
			if !h.Look(string(e.Text)) {
				w.WriteEvent(e)
			}
		}
	}
}

func (w *Win) execute(h EventHandler, cmd string) bool {
	verb, arg := cmd, ""
	if i := strings.IndexAny(verb, " \t"); i >= 0 {
		verb, arg = verb[:i], strings.TrimSpace(verb[i+1:])
	}

	// Look for specific method.
	m := reflect.ValueOf(h).MethodByName("Exec" + verb)
	if !m.IsValid() {
		// Fall back to general Execute.
		return h.Execute(cmd)
	}

	// Found method.
	// Committed to handling the event.
	// All returns below should be return true.

	// Check method signature.
	t := m.Type()
	switch t.NumOut() {
	default:
		w.Errf("bad method %s: too many results", cmd)
		return true
	case 0:
		// ok
	case 1:
		if t.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			w.Errf("bad method %s: return type %v, not error", cmd, t.Out(0))
			return true
		}
	}
	varg := reflect.ValueOf(arg)
	switch t.NumIn() {
	default:
		w.Errf("bad method %s: too many arguments", cmd)
		return true
	case 0:
		if arg != "" {
			w.Errf("%s takes no arguments", cmd)
			return true
		}
	case 1:
		if t.In(0) != varg.Type() {
			w.Errf("bad method %s: argument type %v, not string", cmd, t.In(0))
			return true
		}
	}

	args := []reflect.Value{}
	if t.NumIn() > 0 {
		args = append(args, varg)
	}
	out := m.Call(args)
	var err error
	if len(out) == 1 {
		err, _ = out[0].Interface().(error)
	}
	if err != nil {
		w.Errf("%v", err)
	}

	return true
}

func (w *Win) Selection() string {
	w.Ctl("addr=dot")
	data, err := w.ReadAll("xdata")
	if err != nil {
		w.Err(err.Error())
	}
	return string(data)
}

func (w *Win) SetErrorPrefix(p string) {
	w.errorPrefix = p
}

// Err finds or creates a window appropriate for showing errors related to w
// and then prints msg to that window.
// It adds a final newline to msg if needed.
func (w *Win) Err(msg string) {
	Err(w.errorPrefix, msg)
}

func (w *Win) Errf(format string, args ...interface{}) {
	w.Err(fmt.Sprintf(format, args...))
}

// Err finds or creates a window appropriate for showing errors related to a window titled src
// and then prints msg to that window. It adds a final newline to msg if needed.
func Err(src, msg string) {
	if !strings.HasSuffix(msg, "\n") {
		msg = msg + "\n"
	}
	prefix, _ := path.Split(src)
	if prefix == "/" || prefix == "." {
		prefix = ""
	}
	name := prefix + "+Errors"
	w1 := Show(name)
	if w1 == nil {
		var err error
		w1, err = New()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			w1, err = New()
			if err != nil {
				log.Fatalf("cannot create +Errors window")
			}
		}
		w1.Name("%s", name)
	}
	w1.Addr("$")
	w1.Ctl("dot=addr")
	w1.Fprintf("body", "%s", msg)
	w1.Addr(".,")
	w1.Ctl("dot=addr")
	w1.Ctl("show")
}

// Errf is like Err but accepts a printf-style formatting.
func Errf(src, format string, args ...interface{}) {
	Err(src, fmt.Sprintf(format, args...))
}

// splitFields parses the line into fields.
// Each element of fields must be one of *int, *string or *bool
// which are set to the respective field value.
// Boolean and numeric fields are expected to numbers formatted
// in 11 characters followed by a space.
// String fields are expected to be space terminated.
//
// It returns the rest of line after all the fields have been parsed.
func splitFields(line string, fields ...interface{}) (string, error) {
	n := 0
	for len(fields) > 0 {
		switch f := fields[0].(type) {
		case *int, *bool:
			if len(line) < 12 {
				return "", fmt.Errorf("field %d is too short", n)
			}
			if line[11] != ' ' {
				return "", fmt.Errorf("field %d doesn't terminate in a space", n)
			}
			fn, err := strconv.Atoi(strings.TrimSpace(line[:11]))
			if err != nil {
				return "", fmt.Errorf("field %d is invalid: %v", n, err)
			}
			switch f := f.(type) {
			case *int:
				*f = fn
			case *bool:
				if fn != 0 && fn != 1 {
					return "", fmt.Errorf("field %d should be either 0 or 1", n)
				}
				*f = fn != 0
			}
			line = line[12:]
		case *string:
			i := strings.IndexByte(line, ' ')
			if i == -1 {
				return "", fmt.Errorf("no space found at end of string field %d", n)
			}
			*f = line[:i]
			line = line[i+1:]
		}
		fields = fields[1:]
	}
	return line, nil
}
