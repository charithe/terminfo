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
	ErrSmallFile  = errors.New("terminfo: file too small")
	ErrBadString  = errors.New("terminfo: bad string")
	ErrBigSection = errors.New("terminfo: section too big")
	ErrBadHeader  = errors.New("terminfo: bad header")
)

// header represents a Terminfo file's header.
type header [6]int16

// TODO: convert back to using methods instead of indices.
const (
	magic = iota
	lenNames
	lenBools
	lenNumbers
	lenStrings
	lenTable
)

// lenFile returns the length of the file the header describes in bytes.
func (h header) lenFile() int16 {
	return h.len() + h[lenNames] + h[lenBools] + h[lenNumbers]*2 + h[lenStrings]*2 + h[lenTable]
}

// len returns the length of the header in bytes.
func (h header) len() int16 {
	return int16(len(h) * 2)
}

// littleEndian decodes a int16 starting at i in buf using little-endian byte order.
func littleEndian(i int16, buf []byte) int16 {
	return int16(buf[i+1])<<8 | int16(buf[i])
}

type reader struct {
	pos, ppos int16
	buf       []byte
	ti        *Terminfo
	// TODO: use pointers here or nah?
	h  header
	eh extHeader
}

var readerPool = sync.Pool{
	New: func() interface{} {
		r := new(reader)
		// TODO: What is the max entry size talking about in terminfo(5)?
		r.buf = make([]byte, 4096)
		return r
	},
}

func (r *reader) slice() []byte {
	return r.buf[r.ppos:r.pos]
}

func (r *reader) sliceOff(off int16) []byte {
	r.ppos, r.pos = r.pos, r.pos+off
	return r.slice()
}

func (r *reader) evenBoundary() {
	if r.pos%2 == 1 {
		// Skip extra null byte inserted to align everything on word boundaries.
		r.pos++
	}
}

// TODO read ncurses and find more sanity checks
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
	r.ti = new(Terminfo)
	r.ti.Names = strings.Split(string(r.sliceOff(r.h[lenNames])), "|")
	r.readBools()
	r.evenBoundary()
	r.readNumbers()
	if err = r.readStrings(); err != nil || s <= hl {
		return
	}
	// Extended reader
	r.evenBoundary()
	if err = r.readExtHeader(); err != nil {
		return
	}
	// Read the string names, and then read the caps, much more efficient.
	return
}

type extHeader [5]int16

func (eh extHeader) len() int16 {
	return int16(len(eh) * 2)
}

const (
	lenExtBools = iota
	lenExtNumbers
	lenExtStrings
	lenExtTable
	lasExttOff
)

func (r *reader) readExtHeader() error {
	hbuf := r.sliceOff(r.eh.len())
	for i := 0; i < len(r.eh); i++ {
		n := littleEndian(int16(i*2), hbuf)
		if n < 0 {
			return ErrBadHeader
		}
		r.eh[i] = n
	}
	return nil
}

func (r *reader) readExtBools() {
	for i, b := range r.sliceOff(r.eh[lenExtBools]) {
		if b == 1 {
			r.ti.Bools[i] = true
		}
	}
}

func (r *reader) readHeader() error {
	r.h[magic] = littleEndian(magic, r.buf)
	if r.h[magic] != 0x11A {
		return ErrBadHeader
	}
	for r.pos = 2; r.pos < r.h.len(); r.pos += 2 {
		n := littleEndian(r.pos, r.buf)
		if n < 0 {
			return ErrBadHeader
		}
		r.h[r.pos/2] = n
	}
	return nil
}

func (r *reader) readBools() {
	if r.h[lenBools] >= caps.BoolCount {
		r.h[lenBools] = caps.BoolCount
	}
	for i, b := range r.sliceOff(r.h[lenBools]) {
		if b == 1 {
			r.ti.Bools[i] = true
		}
	}
}

func (r *reader) readNumbers() {
	if r.h[lenNumbers] >= caps.NumberCount {
		r.h[lenNumbers] = caps.NumberCount
	}
	// TODO: is slice really necessary or good?
	nbuf := r.sliceOff(r.h[lenNumbers] * 2)
	for i := int16(0); i < r.h[lenNumbers]; i++ {
		if n := littleEndian(i*2, nbuf); n > -1 {
			r.ti.Numbers[i] = n
		}
	}
}

// readStrings reads the string and string table sections.
func (r *reader) readStrings() error {
	if r.h[lenStrings] >= caps.StringCount {
		r.h[lenStrings] = caps.StringCount
	}
	sbuf := r.sliceOff(r.h[lenStrings] * 2)
	table := r.sliceOff(r.h[lenTable])
	for i := int16(0); i < r.h[lenStrings]; i++ {
		if off := littleEndian(i*2, sbuf); off > -1 {
			j := int(off)
			for {
				if j >= len(table) {
					return ErrBadString
				}
				if table[j] == 0 {
					break
				}
				j++
			}
			r.ti.Strings[i] = string(table[off:j])
		}
	}
	return nil
}
