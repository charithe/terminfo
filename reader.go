package terminfo

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
)

var (
	ErrBadMagic  = errors.New("terminfo: wrong filetype for terminfo file")
	ErrSmallFile = errors.New("terminfo: file too small")
	ErrBadString = errors.New("terminfo: bad string")
)

var readerPool = sync.Pool{
	New: func() interface{} {
		r := new(reader)
		r.buf = make([]byte, 3000)
		return r
	},
}

// TODO extended reader
type reader struct {
	pos, ppos int16
	buf       []byte
	ti        *Terminfo
	h         header
}

func (r *reader) free() {
	r.pos, r.ppos = 0, 0
	r.h = header{}
	readerPool.Put(r)
}

func (r *reader) slice() []byte {
	return r.buf[r.ppos:r.pos]
}

func (r *reader) sliceAdd(i int16) []byte {
	r.ppos, r.pos = r.pos, r.pos+i
	return r.slice()
}

func (r *reader) read(f *os.File) (err error) {
	if err = r.readHeader(f); err != nil {
		return
	}
	r.readNames()
	r.readBools()
	r.readNumbers()
	return r.readStrings()
}

func (r *reader) readHeader(f *os.File) (err error) {
	fi, err := f.Stat()
	if err != nil {
		return
	}
	s := fi.Size()
	if s < int64(len(r.h)) {
		return ErrSmallFile
	}
	if s < int64(len(r.buf)) {
		r.buf = make([]byte, s)
	}
	if _, err = io.ReadFull(f, r.buf); err != nil {
		return
	}
	for i := 0; i < len(r.h); i++ {
		r.h[i] = littleEndian(i*2, r.buf)
	}
	if int(r.h.lenFile()) > len(r.buf) {
		return ErrSmallFile
	}
	if r.h.badMagic() {
		return ErrBadMagic
	}
	return
}

func (r *reader) readNames() {
	r.ppos = r.h.len()
	r.pos = r.ppos + r.h.lenNames()
	r.ti = new(Terminfo)
	r.ti.Names = strings.Split(string(r.slice()), "|")
}

func (r *reader) readBools() {
	for i, b := range r.sliceAdd(r.h.lenBools()) {
		if b == 1 {
			r.ti.BoolCaps[i] = true
		}
	}
	if r.h.skipNull() {
		// Skip extra null byte inserted to align everything on word boundaries.
		r.pos++
	}
}

func (r *reader) readNumbers() {
	nbuf := r.sliceAdd(r.h.lenNumeric())
	for j := 0; j < len(nbuf); j += 2 {
		if n := littleEndian(j, nbuf); n > -1 {
			r.ti.NumericCaps[j/2] = n
		}
	}
}

func (r *reader) readStrings() error {
	// Read the string and string table section.
	sbuf := r.sliceAdd(r.h.lenStrings())
	table := r.buf[r.pos : r.pos+r.h.lenTable()]
	for j := 0; j < len(sbuf); j += 2 {
		if off := littleEndian(j, sbuf); off > -1 {
			x := int(off)
			for ; table[x] != 0; x++ {
				if x+1 >= len(table) {
					return ErrBadString
				}
			}
			r.ti.StringCaps[j/2] = string(table[off:x])
		}
	}
	return nil
}
