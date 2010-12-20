package main

import (
	"bytes"
	"gob"
	"encoding/binary"
	"os"
	"strings"
	"fmt"
	"log"
	"io"
	"sync"
)

func main() {
	s, err := Encode(os.Args[1])
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Printf("encoded: %d bytes\n", len(s))

	fs, err := Decode(s)
	if err != nil {
		log.Exitf("decode: %v\n", err)
		return
	}
	fmt.Printf("all:\n")
	show(fs, "/")
}

func show(fs *FS, path string) {
	f, err := fs.Open(path)
	if err != nil {
		log.Printf("cannot open %s: %v\n", path, err)
		return
	}
	if f.IsDirectory() {
		fmt.Printf("d %s\n", path)
		names, err := f.Readdirnames()
		if err != nil {
			log.Printf("cannot get contents of %s: %v\n", path, err)
			return
		}
		for _, name := range names {
			show(fs, path+"/"+name)
		}
	}else{
		fmt.Printf("- %s\n", path)
		n, err := io.Copy(nullWriter{}, f)
		if err != nil {
			log.Printf("cannot read %s: %v\n", err)
			return
		}
		fmt.Printf("	%d bytes\n", n)
	}
}

type nullWriter struct {}
func (nullWriter) Write(data []byte) (int, os.Error) {
	return len(data), nil
}

// fsWriter represents file system while it's being encoded.
// The gob Encoder writes to the the bytes.Buffer.
type fsWriter struct {
	buf bytes.Buffer
	enc *gob.Encoder
}

// entry is the basic file system structure - it holds
// information on one directory entry.
type entry struct {
	name string		// name of entry.
	offset int			// start of information for this entry.
	dir bool			// is it a directory?
	len int			// length of file (only if it's a file)
}

// FS represents the file system and all its data.
type FS struct {
	mu sync.Mutex
	s string
	root uint32
	dec *gob.Decoder
	rd strings.Reader
}

// A File represents an entry in the file system.
type File struct {
	fs *FS
	rd strings.Reader
	entry *entry
}

// Encode recursively reads the directory at path
// and encodes it into a read only file system
// that can later be read with Decode.
func Encode(path string) (string, os.Error) {
	fs := &fsWriter{}
	fs.enc = gob.NewEncoder(&fs.buf)
	// make sure entry type is encoded first.
	fs.enc.Encode([]entry{})

	e, err := fs.write(path)
	if err != nil {
		return "", err
	}
	if !e.dir {
		return "", os.ErrorString("root must be a directory")
	}
	binary.Write(&fs.buf, binary.LittleEndian, uint32(e.offset))
	return string(fs.buf.Bytes()), nil
}

// write writes path and all its contents to the file system.
func (fs *fsWriter) write(path string) (*entry, os.Error) {
	f, err := os.Open(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if info == nil {
		return nil, err
	}
	if info.IsDirectory() {
		names, err := f.Readdirnames(-1)
		if err != nil {
			return nil, err
		}
		entries := make([]entry, len(names))
		for i, name := range names {
			ent, err := fs.write(path+"/"+name)
			if err != nil {
				return nil, err
			}
			ent.name = name
			entries[i] = *ent
		}
		off := len(fs.buf.Bytes())
		fs.enc.Encode(entries)
		return &entry{offset: off, dir: true}, nil
	}
	off := len(fs.buf.Bytes())
	buf := make([]byte, 8192)
	tot := 0
	for {
		n, _ := f.Read(buf)
		if n == 0 {
			break
		}
		fs.buf.Write(buf[0:n])
		tot += n
	}
	return &entry{offset: off, dir: false, len: tot}, nil
}

// Decode converts a file system as encoded by Encode
// into an FS.
func Decode(s string) (*FS, os.Error) {
	fs := new(FS)
	r := strings.NewReader(s[len(s)-4:])
	if err := binary.Read(r, binary.LittleEndian, &fs.root); err != nil {
		return nil, err
	}
	fs.s = s[0:len(s)-4]
	fs.dec = gob.NewDecoder(&fs.rd)

	// read dummy entry at start to prime the gob types.
	fs.rd = strings.Reader(fs.s)
	if err := fs.dec.Decode(new([]entry)); err != nil {
		return nil, err
	}

	return fs, nil
}

func isSlash(c int) bool {
	return c == '/'
}

// Open opens the named path within fs.
// Paths are slash-separated, with an optional
// slash prefix.
func (fs *FS) Open(path string) (*File, os.Error) {
	p := strings.FieldsFunc(path, isSlash)
	e := &entry{dir: true, offset: int(fs.root)}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, name := range p {
		var err os.Error
		e, err = fs.walk(e, name)
		if err != nil {
			return nil, err
		}
	}
	if e.dir {
		return &File{fs, "", e}, nil
	}
	return &File{
		fs,
		strings.Reader(fs.s[e.offset: e.offset+e.len]),
		e,
	}, nil
}

func (fs *FS) walk(e *entry, name string) (*entry, os.Error) {
	if !e.dir {
		return nil, os.ErrorString("not a directory")
	}
	contents, err := fs.contents(e)
	if err != nil {
		return nil, err
	}
	for i := range contents {
		if contents[i].name == name {
			return &contents[i], nil
		}
	}
	return nil, os.ErrorString("file not found")
}

// IsDirectory returns true if the file represents a directory.
func (f *File) IsDirectory() bool {
	return f.entry.dir
}

// Read reads from a file. It is invalid to call it on a directory.
func (f *File) Read(buf []byte) (int, os.Error) {
	if f.entry.dir {
		return 0, os.ErrorString("cannot read a directory")
	}
	return f.rd.Read(buf)
}

// contents returns all the entries inside a directory.
func (fs *FS) contents(e *entry) (entries []entry, err os.Error) {
	if !e.dir {
		return nil, os.ErrorString("not a directory")
	}
	fs.rd = strings.Reader(fs.s[e.offset:])
	err = fs.dec.Decode(&entries)
	return
}

// Readdirnames returns the names of all the files in
// the File, which must be a directory.
func (f *File) Readdirnames() ([]string, os.Error) {
	f.fs.mu.Unlock()
	defer f.fs.mu.Unlock()
	entries, err := f.fs.contents(f.entry)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names, nil
}
