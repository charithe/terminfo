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
	lenExtOff
)

// lenFile returns the length of the file the header describes in bytes.
func (h header) lenFile() int16 {
	return h[lenNames] +
		h[lenBools] +
		(h[lenNames]+h[lenBools])%2 +
		h[lenNumbers]*2 +
		h[lenStrings]*2 +
		h[lenTable]
}

func (h header) lenExt() int16 {
	return h.len() +
		h[lenExtBools] +
		h[lenExtBools]%2 +
		h[lenExtNumbers]*2 +
		h[lenExtOff]*2 +
		h[lenTable]
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
	// TODO: use pointer here or nah?
	h              header
	extStringTable []byte
	extNameOffPos  int16 // position in the name offsets
	extNameTable   []byte
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

func (r *reader) nextExtName() (string, error) {
	loff := littleEndian(r.extNameOffPos, r.buf)
	lpos := loff
	for {
		if lpos >= r.h[lenTable] {
			return "", ErrBadString
		}
		if r.extNameTable[lpos] == 0 {
			r.extNameOffPos += 2
			return string(r.extNameTable[loff:lpos]), nil
		}
		lpos++
	}
}

// TODO read ncurses and find more sanity checks
func (r *reader) read(f *os.File) (err error) {
	fi, err := f.Stat()
	if err != nil {
		return
	}
	s := int16(fi.Size())
	if s < r.h.len() {
		return ErrSmallFile
	}
	if s > int16(cap(r.buf)) {
		r.buf = make([]byte, s, s*2+1)
	} else {
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
	if s-r.pos < r.h.lenFile() {
		return ErrSmallFile
	}
	r.ti = new(Terminfo)
	r.ti.Names = strings.Split(string(r.sliceOff(r.h[lenNames])), "|")
	r.readBools()
	r.evenBoundary(r.pos)
	r.readNumbers()
	if err = r.readStrings(); err != nil || s <= r.pos {
		return
	}
	// We have extended capabilities.
	r.evenBoundary(r.pos)
	s -= r.pos
	if s < r.h.len() {
		return ErrSmallFile
	}
	if err = r.readHeader(); err != nil {
		return
	}
	if s < r.h.lenExt() {
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
			// TODO error?
			return ErrBadString
		}
		r.h[lenExtStrings]--
		if loff = littleEndian(lpos, r.buf); loff > -1 {
			break
		}
	}
	// Read the capability value.
	r.extStringTable = r.buf[nameOffPos+lenNameOffs:]
	i := loff
	for ; ; i++ {
		if i >= r.h[lenTable] {
			return ErrBadString
		}
		if r.extStringTable[i] == 0 {
			break
		}
	}
	val := string(r.extStringTable[loff:i])
	r.extNameTable = r.extStringTable[i+1:]
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

func (r *reader) readExtBools() error {
	r.ti.ExtBools = make(map[string]bool)
	for _, b := range r.sliceOff(r.h[lenExtBools]) {
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
	nbuf := r.sliceOff(r.h[lenExtNumbers] * 2)
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
OFFLOOP:
	for lastPos := r.pos + r.h[lenExtStrings]*2; r.pos < lastPos; r.pos += 2 {
		if off := littleEndian(r.pos, r.buf); off > -1 {
			for i := int(off); i < len(r.extStringTable); i++ {
				if r.extStringTable[i] == 0 {
					key, err := r.nextExtName()
					if err != nil {
						return err
					}
					r.ti.ExtStrings[key] = string(r.extStringTable[off:i])
					continue OFFLOOP
				}
			}
			return ErrBadString
		}
	}
	return nil
}
