package terminfo

import (
	"errors"
	"io"
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
// It is only 5 int16 because we no don't need to store magic.
type header [5]int16

// What each int16 means in the standard format.
const (
	lenNames = iota
	lenBools
	lenNumbers
	lenStrings
	lenTable
)

// What each int16 means in the extended format.
// lenTable is the same in both so it was not repeated here.
const (
	lenExtBools = iota
	lenExtNumbers
	lenExtStrings
	lenExtOff
)

// lenCaps returns the length of all of the capabilies in bytes.
func (h header) lenCaps() int16 {
	return h[lenNames] +
		h[lenBools] +
		(h[lenNames]+h[lenBools])%2 +
		h[lenNumbers]*2 +
		h[lenStrings]*2 +
		h[lenTable]
}

// lenExtCaps returns the length of all the extended capabilities in bytes.
func (h header) lenExtCaps() int16 {
	return h[lenExtBools] +
		h[lenExtBools]%2 +
		h[lenExtNumbers]*2 +
		h[lenExtOff]*2 +
		h[lenTable]
}

// len returns the length of the header in bytes.
func (h header) lenBytes() int16 {
	return int16(len(h) * 2)
}

// littleEndian decodes a int16 starting at i in buf using little-endian byte order.
func littleEndian(i int16, buf []byte) int16 {
	return int16(buf[i+1])<<8 | int16(buf[i])
}

type reader struct {
	pos            int16
	extNameOffPos  int16 // position in the name offsets
	h              header
	buf            []byte
	extStringTable []byte
	extNameTable   []byte
	ti             *Terminfo
}

var readerPool = sync.Pool{
	New: func() interface{} {
		r := new(reader)
		// TODO: What is the max entry size talking about in terminfo(5)?
		r.buf = make([]byte, 4096)
		return r
	},
}

func (r *reader) sliceNext(off int16) []byte {
	// Just use off as ppos.
	off, r.pos = r.pos, r.pos+off
	return r.buf[off:r.pos]
}

func (r *reader) evenBoundary(n int16) {
	if n%2 == 1 {
		// Skip extra null byte inserted to align everything on word boundaries.
		r.pos++
	}
}

// nextNull returns the position of the next null byte in buf.
// It is used to find the end of null terminated strings.
func nextNull(off int16, buf []byte) (int16, error) {
	for pos := off; ; pos++ {
		if pos >= int16(len(buf)) {
			return 0, ErrBadString
		}
		if buf[pos] == 0 {
			return pos, nil
		}
	}
}

// TODO read ncurses and find more sanity checks
func (r *reader) read(f *os.File) (err error) {
	fi, err := f.Stat()
	if err != nil {
		return
	}
	s, hl := int16(fi.Size()), r.h.lenBytes()
	if s < hl {
		return ErrSmallFile
	}
	if s > int16(cap(r.buf)) {
		r.buf = make([]byte, s, s*2+1)
	} else if s > int16(len(r.buf)) {
		r.buf = r.buf[:s]
	}
	if _, err = io.ReadAtLeast(f, r.buf, int(s)); err != nil {
		return
	}
	if littleEndian(0, r.buf) != 0x11A {
		return ErrBadHeader
	}
	r.pos = 2 // skip magic
	if err = r.readHeader(); err != nil {
		return
	}
	if s-r.pos < r.h.lenCaps() {
		return ErrSmallFile
	}
	r.ti = new(Terminfo)
	r.ti.Names = strings.Split(string(r.sliceNext(r.h[lenNames])), "|")
	r.readBools()
	r.evenBoundary(r.pos)
	r.readNumbers()
	if err = r.readStrings(); err != nil || s <= r.pos {
		return
	}
	// We have extended capabilities.
	r.evenBoundary(r.pos)
	s -= r.pos
	if s < hl {
		return ErrSmallFile
	}
	if err = r.readHeader(); err != nil {
		return
	}
	if s-hl < r.h.lenExtCaps() {
		return ErrSmallFile
	}
	if err = r.setExtNameTable(); err != nil {
		return
	}
	if err = r.readExtBools(); err != nil {
		return
	}
	r.evenBoundary(r.h[lenExtBools])
	if err = r.readExtNumbers(); err != nil {
		return
	}
	return r.readExtStrings()
}

func (r *reader) readHeader() error {
	hbuf := r.sliceNext(r.h.lenBytes())
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
	for i, b := range r.sliceNext(r.h[lenBools]) {
		if b == 1 {
			r.ti.Bools[i] = true
		}
	}
}

func (r *reader) readNumbers() {
	if r.h[lenNumbers] >= caps.NumberCount {
		r.h[lenNumbers] = caps.NumberCount
	}
	nbuf := r.sliceNext(r.h[lenNumbers] * 2)
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
	sbuf := r.sliceNext(r.h[lenStrings] * 2)
	table := r.sliceNext(r.h[lenTable])
	for i := int16(0); i < r.h[lenStrings]; i++ {
		if off := littleEndian(i*2, sbuf); off > -1 {
			end, err := nextNull(off, table)
			if err != nil {
				return err
			}
			r.ti.Strings[i] = string(table[off:end])
		}
	}
	return nil
}

func (r *reader) setExtNameTable() error {
	// Beginning of name offsets.
	nameOffPos := r.pos +
		r.h[lenExtBools] +
		r.h[lenExtBools]%2 +
		r.h[lenExtNumbers]*2 +
		r.h[lenExtStrings]*2
	lenNameOffs := (r.h[lenExtOff] - r.h[lenExtStrings]) * 2
	// Find last string offset.
	lpos, loff := nameOffPos, int16(0)
	for {
		lpos -= 2
		if lpos < r.pos {
			return ErrBadString
		}
		// TODO no! stop this:
		r.h[lenExtStrings]--
		if loff = littleEndian(lpos, r.buf); loff > -1 {
			break
		}
	}
	// Read the capability value.
	r.extStringTable = r.buf[nameOffPos+lenNameOffs:]
	end, err := nextNull(loff, r.extStringTable)
	if err != nil {
		return err
	}
	val := string(r.extStringTable[loff:end])
	r.extNameTable = r.extStringTable[end+1:]
	r.extStringTable = r.extStringTable[:loff]
	r.extNameOffPos = lpos + lenNameOffs
	key, err := r.nextExtName()
	if err != nil {
		// TODO error?
		return ErrBadString
	}
	r.ti.ExtStrings = make(map[string]string)
	r.ti.ExtStrings[key] = val
	// Set extNameOffPos to the start of the name offset section.
	r.extNameOffPos = nameOffPos
	return nil
}

func (r *reader) nextExtName() (string, error) {
	off := littleEndian(r.extNameOffPos, r.buf)
	end, err := nextNull(off, r.extNameTable)
	if err != nil {
		return "", err
	}
	r.extNameOffPos += 2
	return string(r.extNameTable[off:end]), nil
}

func (r *reader) readExtBools() error {
	r.ti.ExtBools = make(map[string]bool)
	for _, b := range r.sliceNext(r.h[lenExtBools]) {
		if b == 1 {
			key, err := r.nextExtName()
			if err != nil {
				return err
			}
			r.ti.ExtBools[key] = true
		}
	}
	return nil
}

func (r *reader) readExtNumbers() error {
	r.ti.ExtNumbers = make(map[string]int16)
	nbuf := r.sliceNext(r.h[lenExtNumbers] * 2)
	for i := int16(0); i < r.h[lenExtNumbers]; i++ {
		if n := littleEndian(i*2, nbuf); n > -1 {
			key, err := r.nextExtName()
			if err != nil {
				return err
			}
			r.ti.ExtNumbers[key] = n
		}
	}
	return nil
}

func (r *reader) readExtStrings() error {
	for lpos := r.pos + r.h[lenExtStrings]*2; r.pos < lpos; r.pos += 2 {
		if off := littleEndian(r.pos, r.buf); off > -1 {
			end, err := nextNull(off, r.extStringTable)
			if err != nil {
				return err
			}
			key, err := r.nextExtName()
			if err != nil {
				return err
			}
			r.ti.ExtStrings[key] = string(r.extStringTable[off:end])
		}
	}
	return nil
}
