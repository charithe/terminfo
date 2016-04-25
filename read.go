package terminfo

import (
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/nhooyr/terminfo/caps"
)

var (
	ErrBadMagic  = errors.New("terminfo: wrong filetype for terminfo file")
	ErrSmallFile = errors.New("terminfo: file too small")
	ErrBadString = errors.New("terminfo: bad string")
)

// header represents a Terminfo file's header.
type header [6]int16

// badMagic returns false if the correct magic number is set on the header and true otherwise.
func (h header) badMagic() bool {
	if h[0] == 0x11A {
		return false
	}
	return true
}

// lenNames returns the length of name section
func (h header) lenNames() int16 {
	return h[1]
}

// lenBools returns the length of boolean section
func (h header) lenBools() int16 {
	return h[2]
}

// lenNumbers returns the length of numbers section
func (h header) lenNumbers() int16 {
	return h[3] * 2 // stored as number of int16
}

// lenStrings returns the length of string section
func (h header) lenStrings() int16 {
	return h[4] * 2 // stored as number of int16
}

// lenTable returns the length of string table in bytes.
func (h header) lenTable() int16 {
	return h[5]
}

// lenFile returns the length of the file the header describes.
func (h header) lenFile() int16 {
	return h[1] + h[2] + h[3] + h[4] + h[5]
}

// len returns the length of the header in bytes.
func (h header) len() int16 {
	return int16(len(h) * 2)
}

// littleEndian decodes a int16 starting at i in buf using little-endian byte order.
func littleEndian(i int, buf []byte) int16 {
	return int16(buf[i+1])<<8 | int16(buf[i])
}

type reader struct {
	pos, ppos int16
	buf       []byte
	ti        *Terminfo
	h         header
}

var readerPool = sync.Pool{
	New: func() interface{} {
		r := new(reader)
		r.buf = make([]byte, 3000)
		return r
	},
}

func (r *reader) free() {
	r.pos, r.ppos = 0, 0
	r.h = header{}
	readerPool.Put(r)
}

func (r *reader) slice() []byte {
	return r.buf[r.ppos:r.pos]
}

func (r *reader) sliceOff(off int16) []byte {
	r.ppos, r.pos = r.pos, r.pos+off
	return r.slice()
}

func (r *reader) read(f *os.File) (err error) {
	fi, err := f.Stat()
	if err != nil {
		return
	}
	s := int(fi.Size())
	if s < len(r.h) {
		return ErrSmallFile
	}
	if s > len(r.buf) {
		r.buf = make([]byte, s*2+1)
	}
	if _, err = io.ReadAtLeast(f, r.buf, s); err != nil {
		return
	}
	if err = r.readHeader(); err != nil {
		return
	}
	hl := int(r.h.lenFile())
	if s < hl {
		return ErrSmallFile
	}
	r.readNames()
	r.readBools()
	r.readNumbers()
	if err = r.readStrings(); err != nil {
		return
	}
	if s > hl {
		if hl%2 == 1 {
			r.pos++
		}
		log.Println("extended")
	}
	return nil
}

func (r *reader) readHeader() error {
	for i := 0; i < len(r.h); i++ {
		r.h[i] = littleEndian(i*2, r.buf)
	}
	if r.h.badMagic() {
		return ErrBadMagic
	}
	return nil
}

func (r *reader) readNames() {
	r.ppos = r.h.len()
	r.pos = r.ppos + r.h.lenNames()
	r.ti = new(Terminfo)
	r.ti.Names = strings.Split(string(r.slice()), "|")
}

func (r *reader) readBools() {
	if r.h.lenBools() > caps.BoolCount {
		return
	}
	for i, b := range r.sliceOff(r.h.lenBools()) {
		if b == 1 {
			r.ti.Bools[i] = true
		}
	}
	if (r.h.lenNames()+r.h.lenBools())%2 == 1 {
		// Skip extra null byte inserted to align everything on word boundaries.
		r.pos++
	}
}

func (r *reader) readNumbers() {
	nbuf := r.sliceOff(r.h.lenNumbers())
	for j := 0; j < len(nbuf); j += 2 {
		if n := littleEndian(j, nbuf); n > -1 {
			r.ti.Numbers[j/2] = n
		}
	}
}

func (r *reader) readStrings() error {
	// Read the string and string table section.
	sbuf := r.sliceOff(r.h.lenStrings())
	table := r.sliceOff(r.h.lenTable())
	for j := 0; j < len(sbuf); j += 2 {
		if off := littleEndian(j, sbuf); off > -1 {
			x := int(off)
			for ; table[x] != 0; x++ {
				if x+1 >= len(table) {
					return ErrBadString
				}
			}
			r.ti.Strings[j/2] = string(table[off:x])
		}
	}
	return nil
}
