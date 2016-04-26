package terminfo

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"strconv"
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
type header [5]int16

// No need to store magic.
const (
	lenNames = iota
	lenBools
	lenNumbers
	lenStrings
	lenTable
)

const (
	lenExtBools = iota
	lenExtNumbers
	lenExtStrings
	lenExtTable
	lastOff
)

// lenFile returns the length of the file the header describes in bytes.
func (h header) lenFile() int16 {
	return 2 + h.len() + h[lenNames] + h[lenBools] + h[lenNumbers]*2 + h[lenStrings]*2 + h[lenTable]
}

func (h header) lenExt() int16 {
	return h[lenExtBools] + h[lenExtNumbers]*2 + h[lenExtStrings]*2 + h[lenExtTable]
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
	pos int16
	buf []byte
	ti  *Terminfo
	// TODO: use pointers here or nah?
	h header
}

var readerPool = sync.Pool{
	New: func() interface{} {
		r := new(reader)
		// TODO: What is the max entry size talking about in terminfo(5)?
		r.buf = make([]byte, 4096)
		return r
	},
}

func (r *reader) sliceOff(off int16) []byte {
	// Just use off as ppos.
	off, r.pos = r.pos, r.pos+off
	return r.buf[off:r.pos]
}

func (r *reader) evenBoundary(i int16) {
	if i%2 == 1 {
		// Skip extra null byte inserted to align everything on word boundaries.
		r.pos++
	}
}

// TODO read ncurses and find more sanity checks
func (r *reader) read(f *os.File) (err error) {
	log.SetFlags(0)
	fi, err := f.Stat()
	if err != nil {
		return
	}
	s := int(fi.Size())
	if s < len(r.h) {
		return ErrSmallFile
	}
	if s > cap(r.buf) {
		r.buf = make([]byte, s, s*2+1)
	} else {
		r.buf = r.buf[:s]
	}
	if _, err = io.ReadAtLeast(f, r.buf, s); err != nil {
		return
	}
	if littleEndian(0, r.buf) != 0x11A {
		return ErrBadHeader
	}
	r.pos = 2 // skip magic
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
	r.evenBoundary(r.pos)
	r.readNumbers()
	if err = r.readStrings(); err != nil || s <= hl {
		return
	}
	// Extended reading
	r.evenBoundary(r.pos)
	if err = r.readHeader(); err != nil {
		return
	}
	log.Println(r.h)
	// IGNORE THE REST
	return
	r.ti.ExtBools = make(map[string]bool)
	for i, b := range r.sliceOff(r.h[lenExtBools]) {
		if b == 1 {
			r.ti.ExtBools[strconv.Itoa(i)] = true
		}
	}
	r.evenBoundary(r.h[lenExtBools])
	nbuf := r.sliceOff(r.h[lenExtNumbers] * 2)
	for i := int16(0); i < r.h[lenExtNumbers]; i++ {
		if n := littleEndian(i*2, nbuf); n > -1 {
			r.ti.ExtNumbers[strconv.Itoa(int(i))] = n
		}
	}
	sbuf := r.sliceOff(r.h[lenExtStrings] * 2)
	log.Printf("%q\n\n", sbuf)
	table := r.sliceOff(r.h[lenExtTable] + r.h[lastOff])
	log.Printf("%q\n\n", table)
	log.Println(bytes.Count(table, []byte{0}))
	for i := int16(0); i < r.h[lenExtStrings]; i++ {
		if off := littleEndian(i*2, sbuf); off > -1 {
			j := off
			for {
				if j >= r.h[lenExtTable]+r.h[lastOff] {
					return ErrBadString
				}
				if table[j] == 0 {
					break
				}
				j++
			}
			log.Printf("%q", string(table[off:j]))
		}
	}
	return
}

func (r *reader) readHeader() error {
	hbuf := r.sliceOff(r.h.len())
	for i := 0; i < len(r.h); i++ {
		n := littleEndian(int16(i*2), hbuf)
		if n < 0 {
			return ErrBadHeader
		}
		r.h[i] = n
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
			j := off
			for {
				if j >= r.h[lenTable] {
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
