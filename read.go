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
func littleEndian(i int, buf []byte) int16 {
	return int16(buf[i+1])<<8 | int16(buf[i])
}

// TODO still need to work on extended reader.
type reader struct {
	pos, ppos int16
	buf       []byte
	ti        *Terminfo
	h         header
}

var readerPool = sync.Pool{
	New: func() interface{} {
		r := new(reader)
		// TODO: What is the max entry size talking about in terminfo(5)?
		r.buf = make([]byte, 4096)
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
	r.h[magic] = littleEndian(magic, r.buf)
	if r.h[magic] != 0x11A {
		return ErrBadHeader
	}
	for i := 1; i < len(r.h); i++ {
		j := littleEndian(i*2, r.buf)
		if j < 0 {
			return ErrBadHeader
		}
		r.h[i] = j
	}
	return nil
}

func (r *reader) readNames() {
	r.ppos = r.h.len()
	r.pos = r.ppos + r.h[lenNames]
	r.ti = new(Terminfo)
	// The string is null terminated but, go handles it fine.
	r.ti.Names = strings.Split(string(r.slice()), "|")
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
	if (r.h[lenNames]+r.h[lenBools])%2 == 1 {
		// Skip extra null byte inserted to align everything on word boundaries.
		r.pos++
	}
}

func (r *reader) readNumbers() {
	if r.h[lenNumbers] >= caps.NumberCount {
		r.h[lenNumbers] = caps.NumberCount
	}
	nbuf := r.sliceOff(r.h[lenNumbers] * 2)
	for j := 0; j < len(nbuf); j += 2 {
		if n := littleEndian(j, nbuf); n > -1 {
			r.ti.Numbers[j/2] = n
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
	for j := 0; j < len(sbuf); j += 2 {
		if off := littleEndian(j, sbuf); off > -1 {
			x := int(off)
			for {
				if x >= len(table) {
					return ErrBadString
				}
				if table[x] == 0 {
					break
				}
				x++
			}
			r.ti.Strings[j/2] = string(table[off:x])
		}
	}
	return nil
}
