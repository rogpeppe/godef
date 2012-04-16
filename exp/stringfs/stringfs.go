// The stringfs package provides a way to recursively encode the
// data in a directory as a string, and to extract the contents later.
package stringfs

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"os"
	"strings"
	"sync"
)

// The file system encoding uses gob to encode the
// metadata.
// It takes advantage of the fact that gob encodes all
// its types up front, which means we can decode gob
// package out of order as long as we know that the
// relevant types have been received first.
//
// The first item in the file system is a dummy type
// representing an array of directory entries.
// The final four bytes hold the offset of the start
// of the root directory (note that this uses regular
// non-variable-width encoding, so that we know
// exactly where it is).
// Each file entry is directly encoded as the bytes
// of the file, not using gob.
// Each directory entry is encoded as an array of
// entry types.

// entry is the basic file system structure - it holds
// information on one directory entry.
type entry struct {
	name   string // name of entry.
	offset int    // start of information for this entry.
	dir    bool   // is it a directory?
	len    int    // length of file (only if it's a file)
}

// fsWriter represents file system while it's being encoded.
// The gob Encoder writes to the the bytes.Buffer.
type fsWriter struct {
	buf bytes.Buffer
	enc *gob.Encoder
}

// FS represents the file system and all its data.
type FS struct {
	mu   sync.Mutex
	s    string
	root uint32       // offset of root directory.
	dec  *gob.Decoder // primed decoder, reading from &rd.
	rd   strings.Reader
}

// A File represents an entry in the file system.
type File struct {
	fs    *FS
	rd    strings.Reader
	entry *entry
}

// Encode recursively reads the directory at path
// and encodes it into a read only file system
// that can later be read with Decode.
func Encode(path string) (string, error) {
	fs := &fsWriter{}
	fs.enc = gob.NewEncoder(&fs.buf)
	// make sure entry type is encoded first.
	fs.enc.Encode([]entry{})

	e, err := fs.write(path)
	if err != nil {
		return "", err
	}
	if !e.dir {
		return "", errors.New("root must be a directory")
	}
	binary.Write(&fs.buf, binary.LittleEndian, uint32(e.offset))
	return string(fs.buf.Bytes()), nil
}

// write writes path and all its contents to the file system.
func (fs *fsWriter) write(path string) (*entry, error) {
	f, err := os.Open(path)
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
			ent, err := fs.write(path + "/" + name)
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
func Decode(s string) (*FS, error) {
	fs := new(FS)
	r := strings.NewReader(s[len(s)-4:])
	if err := binary.Read(r, binary.LittleEndian, &fs.root); err != nil {
		return nil, err
	}
	fs.s = s[0 : len(s)-4]
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
func (fs *FS) Open(path string) (*File, error) {
	p := strings.FieldsFunc(path, isSlash)
	e := &entry{dir: true, offset: int(fs.root)}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, name := range p {
		var err error
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
		strings.Reader(fs.s[e.offset : e.offset+e.len]),
		e,
	}, nil
}

func (fs *FS) walk(e *entry, name string) (*entry, error) {
	if !e.dir {
		return nil, errors.New("not a directory")
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
	return nil, errors.New("file not found")
}

// IsDirectory returns true if the file represents a directory.
func (f *File) IsDirectory() bool {
	return f.entry.dir
}

// Read reads from a file. It is invalid to call it on a directory.
func (f *File) Read(buf []byte) (int, error) {
	if f.entry.dir {
		return 0, errors.New("cannot read a directory")
	}
	return f.rd.Read(buf)
}

// contents returns all the entries inside a directory.
func (fs *FS) contents(e *entry) (entries []entry, err error) {
	if !e.dir {
		return nil, errors.New("not a directory")
	}
	fs.rd = strings.Reader(fs.s[e.offset:])
	err = fs.dec.Decode(&entries)
	return
}

// Readdirnames returns the names of all the files in
// the File, which must be a directory.
func (f *File) Readdirnames() ([]string, error) {
	f.fs.mu.Lock()
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
