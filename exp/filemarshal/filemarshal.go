// 
package filemarshal

import (
	"code.google.com/p/rog-go/typeapply"
	"errors"
	"io"
	"io/ioutil"
	"os"
)

// A File holds on-disk storage.
type File struct {
	Name string // The name of the file.
	file *os.File
}

// NewFile creates a new file referring to f,
// which should be seekable (i.e. not a pipe
// or network connection).
func NewFile(f *os.File) *File {
	return &File{Name: f.Name(), file: f}
}

// File returns the backing file of f.
func (f *File) File() *os.File {
	return f.file
}

// Encoder represents a encoding method.
// Examples include gob.Encoder and json.Encoder.
type Encoder interface {
	// Encode writes an encoded version of x.
	Encode(x interface{}) error
}

// Decoder represents a decoding method.
// Examples include gob.Decoder and json.Decoder.
type Decoder interface {
	Decode(x interface{}) error
}

type encoder struct {
	enc Encoder
}

type decoder struct {
	dec Decoder
}

//  Encode writes a representation of x to the encoder.  We have to be
//  careful about how we do this, because the data type that we decode
//  into does not necessarily match the encoded data type.  Fields may
//  occur in a different order, and some fields may not occur in the
//  final type.  To cope with this, we assume that each Name in an
//  os.File is unique.  We first encode x itself, followed by a list of
//  the file names within it, followed by the data from all those files,
//  in list order, as a sequence of byte slices, terminated with a
//  zero-length slice.
// 
//  When the decoder decodes the value, it can then associate the correct
//  item in the data structure with the correct file stream.
func (enc encoder) Encode(x interface{}) error {
	// TODO some kind of signature so that we can be more
	// robust if we try to Decode a stream that has not
	// been encoded with filemarshal?

	err := enc.enc.Encode(x)
	if err != nil {
		return err
	}
	files := make(map[string]*File)
	var names []string
	typeapply.Do(func(f *File) {
		if f.Name != "" && files[f.Name] == nil {
			names = append(names, f.Name)
			files[f.Name] = f
		}
	},
		x)
	err = enc.enc.Encode(names)
	if err != nil {
		return err
	}
	buf := make([]byte, 8192)
	for _, name := range names {
		off := int64(0)
		f := files[name]
		for {
			n, err := f.file.ReadAt(buf, off)
			if n > 0 {
				err = enc.enc.Encode(buf[0:n])
				if err != nil {
					return err
				}
			}
			if err != nil {
				break
			}
			off += int64(n)
		}
		err = enc.enc.Encode([]byte{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (dec decoder) Decode(x interface{}) error {
	err := dec.dec.Decode(x)
	if err != nil {
		return err
	}
	var names []string
	err = dec.dec.Decode(&names)
	if err != nil {
		return err
	}
	files := make(map[string][]*File)
	typeapply.Do(func(f *File) {
		if f != nil && f.Name != "" {
			files[f.Name] = append(files[f.Name], f)
		}
	},
		x)
	for _, name := range names {
		var out io.WriteSeeker
		if files[name] == nil {
			out = nullWriter{}
		} else {
			samefiles := files[name]
			f := samefiles[0]
			if f == nil {
				return errors.New("file not found in manifest")
			}
			f.file, err = ioutil.TempFile("", "filemarshal")
			if err != nil {
				return err
			}
			f.Name = f.file.Name()
			for _, g := range samefiles[1:] {
				*g = *f
			}
			out = f.file
		}
		for {
			buf := []byte(nil)
			err = dec.dec.Decode(&buf)
			if err != nil {
				return err
			}
			if len(buf) == 0 {
				break
			}
			_, err = out.Write(buf)
			if err != nil {
				return err
			}
		}
		out.Seek(0, 0)
	}
	return nil
}

type nullWriter struct{}

func (nullWriter) Write(buf []byte) (int, error) {
	return len(buf), nil
}
func (nullWriter) Seek(int64, int) (int64, error) {
	return 0, nil
}

// NewEncoder returns a new Encoder that will encode any *File instance
// that it finds within values passed to Encode.  The resulting encoded
// stream must be decoded with a decoder created by NewDecoder.
func NewEncoder(enc Encoder) Encoder {
	return encoder{enc}
}

// NewDecoder returns a new Decoder that can decode an encoding stream
// produced by an encoder created with NewEncoder.  Any File data gets
// written to disk rather than being stored in memory.  When a File is
// decoded, its file name will have changed to the name of a local temporary
// file holding the same data.
func NewDecoder(dec Decoder) Decoder {
	return decoder{dec}
}
