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
// It is only 5 shorts because we don't need to store magic.
type header [5]int16

// The magic number of terminfo files.
const magic = 0x11a

// What each short means in the standard format.
const (
	lenNames   = iota // bytes
	lenBools          // bytes
	lenNumbers        // shorts
	lenStrings        // shorts
	lenTable          // bytes
)

// What each short means in the extended format.
// lenTable is the same in both so it was not repeated here.
const (
	lenExtBools   = iota // bytes
	lenExtNumbers        // shorts
	lenExtStrings        // shorts
	lenExtOff            // shorts
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

// lenBytes returns the length of the header in bytes.
func (h header) lenBytes() int16 {
	return int16(len(h) * 2)
}

// TODO rename unmarshaller or decoder?
type reader struct {
	pos            int16
	extNameOffPos  int16 // position in the name offsets
	h              header
	buf            []byte
	extStringTable []byte
	extNameTable   []byte
	ti             *Terminfo
}

// TODO is this a premature optimization?
var readerPool = sync.Pool{
	New: func() interface{} {
		r := new(reader)
		// TODO: What is the max entry size talking about in terminfo(5)?
		r.buf = make([]byte, 4096)
		return r
	},
}

// sliceNext slices the next off bytes of r.buf.
// It also increments r.pos by off.
func (r *reader) sliceNext(off int16) []byte {
	// Just use off as ppos.
	off, r.pos = r.pos, r.pos+off
	return r.buf[off:r.pos]
}

// evenBoundary checks if we are on an uneven word boundary.
// If so, it will skip the next byte, which should be a null.
func (r *reader) evenBoundary() {
	if r.pos%2 == 1 {
		r.pos++
	}
}

// read reads the terminfo file from f.
func (r *reader) read(f *os.File) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	s, hl := int16(fi.Size()), r.h.lenBytes()
	// Add 2 extra for the magic.
	if s < hl+2 {
		return ErrSmallFile
	}
	if s > int16(cap(r.buf)) {
		r.buf = make([]byte, s, s*2+1)
	} else {
		r.buf = r.buf[:s]
	}
	if _, err = io.ReadAtLeast(f, r.buf, int(s)); err != nil {
		return err
	}
	if littleEndian(0, r.buf) != magic {
		return ErrBadHeader
	}
	// Skip magic.
	r.pos = 2
	if err = r.readHeader(); err != nil {
		return err
	}
	if s-r.pos < r.h.lenCaps() {
		return ErrSmallFile
	}
	if r.h[lenBools] > caps.BoolCount ||
		r.h[lenNumbers] > caps.NumberCount ||
		r.h[lenStrings] > caps.StringCount {
		return ErrBadHeader
	}
	r.ti = new(Terminfo)
	r.ti.Names = strings.Split(string(r.sliceNext(r.h[lenNames])), "|")
	r.readBools()
	r.evenBoundary()
	r.readNumbers()
	if err = r.readStrings(); err != nil || s <= r.pos {
		return err
	}
	// We have extended capabilities.
	r.evenBoundary()
	if s -= r.pos; s < hl {
		return ErrSmallFile
	}
	if err = r.readHeader(); err != nil {
		return err
	}
	if r.h[lenExtBools]+r.h[lenExtNumbers]+r.h[lenExtStrings]*2 != r.h[lenExtOff] {
		return ErrBadHeader
	}
	if s-hl < r.h.lenExtCaps() {
		return ErrSmallFile
	}
	if err = r.setExtNameTable(); err != nil {
		return err
	}
	if err = r.readExtBools(); err != nil {
		return err
	}
	r.evenBoundary()
	if err = r.readExtNumbers(); err != nil {
		return err
	}
	return r.readExtStrings()
}

// readHeader reads the terminfo header.
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

// readBools reads the boolean section.
func (r *reader) readBools() {
	for i, b := range r.sliceNext(r.h[lenBools]) {
		if b == 1 {
			r.ti.Bools[i] = true
		}
	}
}

// readNumbers reads the numeric section.
func (r *reader) readNumbers() {
	nbuf := r.sliceNext(r.h[lenNumbers] * 2)
	for i := int16(0); i < r.h[lenNumbers]; i++ {
		if n := littleEndian(i*2, nbuf); n > -1 {
			r.ti.Numbers[i] = n
		}
	}
}

// readStrings reads the string and string table sections.
func (r *reader) readStrings() error {
	sbuf := r.sliceNext(r.h[lenStrings] * 2)
	table := r.sliceNext(r.h[lenTable])
	for i := int16(0); i < r.h[lenStrings]; i++ {
		if off := littleEndian(i*2, sbuf); off > -1 {
			end := indexNull(off, table)
			if end == -1 {
				return ErrBadString
			}
			r.ti.Strings[i] = string(table[off:end])
		}
	}
	return nil
}

// setExtNameTable splits the string table into a string table and a name table.
// This allows us to read the capabilities and their names concurrently.
func (r *reader) setExtNameTable() error {
	// The following works because
	// r.h[lenExtOff] == r.h[lenExtBools]+r.h[lenExtNumbers]+r.h[lenExtStrings]*2.
	// See the check in r.read.
	r.extNameOffPos = r.pos +
		r.h[lenExtBools]%2 +
		r.h[lenExtNumbers] +
		r.h[lenExtOff]
	lenNameOffs := (r.h[lenExtOff] - r.h[lenExtStrings]) * 2
	// Find last string offset.
	vpos, voff := r.extNameOffPos, int16(0)
	for {
		vpos -= 2
		if vpos < r.pos {
			return ErrBadString
		}
		r.h[lenExtStrings]--
		if voff = littleEndian(vpos, r.buf); voff > -1 {
			break
		}
	}
	// Read the capability value.
	r.extStringTable = r.buf[r.extNameOffPos+lenNameOffs:]
	vend := indexNull(voff, r.extStringTable)
	if vend == -1 {
		return ErrBadString
	}
	// The rest is the name table
	r.extNameTable = r.extStringTable[vend+1:]
	// Find the capability's name in the name table.
	koff := littleEndian(vpos+lenNameOffs, r.buf)
	kend := indexNull(koff, r.extNameTable)
	if kend == -1 {
		return ErrBadString
	}
	r.ti.ExtStrings = make(map[string]string)
	r.ti.ExtStrings[string(r.extNameTable[koff:kend])] = string(r.extStringTable[voff:vend])
	// Truncate the string table to only values and before the last value.
	r.extStringTable = r.extStringTable[:voff]
	// Truncate the name table to before the last name.
	r.extNameTable = r.extNameTable[:koff]
	return nil
}

// nextExtName gets the offset and ending of the next capability name.
func (r *reader) nextExtName() (off, end int16) {
	off = littleEndian(r.extNameOffPos, r.buf)
	r.extNameOffPos += 2
	end = indexNull(off, r.extNameTable)
	return
}

// readExtBools reads the extended boolean section.
func (r *reader) readExtBools() error {
	r.ti.ExtBools = make(map[string]bool)
	for _, b := range r.sliceNext(r.h[lenExtBools]) {
		off, end := r.nextExtName()
		if end == -1 {
			return ErrBadString
		}
		if b == 1 {
			r.ti.ExtBools[string(r.extNameTable[off:end])] = true
		}
	}
	return nil
}

// readExtNumbers reads the extended numeric section.
func (r *reader) readExtNumbers() error {
	r.ti.ExtNumbers = make(map[string]int16)
	nbuf := r.sliceNext(r.h[lenExtNumbers] * 2)
	for i := int16(0); i < r.h[lenExtNumbers]; i++ {
		off, end := r.nextExtName()
		if end == -1 {
			return ErrBadString
		}
		if n := littleEndian(i*2, nbuf); n > -1 {
			r.ti.ExtNumbers[string(r.extNameTable[off:end])] = n
		}
	}
	return nil
}

// readExtStrings reads the extended string and string table sections.
func (r *reader) readExtStrings() error {
	// lpos is the last position.
	for lpos := r.pos + r.h[lenExtStrings]*2; r.pos < lpos; r.pos += 2 {
		koff, kend := r.nextExtName()
		if kend == -1 {
			return ErrBadString
		}
		if voff := littleEndian(r.pos, r.buf); voff > -1 {
			vend := indexNull(voff, r.extStringTable)
			if vend == -1 {
				return ErrBadString
			}
			r.ti.ExtStrings[string(r.extNameTable[koff:kend])] = string(r.extStringTable[voff:vend])
		}
	}
	return nil
}

// littleEndian decodes a short starting at i in buf using little-endian byte order.
func littleEndian(i int16, buf []byte) int16 {
	return int16(buf[i+1])<<8 | int16(buf[i])
}

// indexNull returns the position of the next null byte in buf.
// It is used to find the end of null terminated strings.
func indexNull(off int16, buf []byte) int16 {
	for ; buf[off] != 0; off++ {
		if off >= int16(len(buf)) {
			return -1
		}
	}
	return off
}
