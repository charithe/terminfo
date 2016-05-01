package terminfo

import (
	"errors"
	"strings"

	"github.com/nhooyr/terminfo/caps"
)

// Decoding errors.
var (
	ErrSmallFile  = errors.New("terminfo: file too small")
	ErrBadString  = errors.New("terminfo: bad string")
	ErrBigSection = errors.New("terminfo: section too big")
	ErrBadHeader  = errors.New("terminfo: bad header")
)

// decoder represents the state while decoding a terminfo file.
type decoder struct {
	pos            int16
	extNameOffPos  int16 // position in the name offsets
	h              header
	buf            []byte
	extStringTable []byte
	extNameTable   []byte
	ti             *Terminfo
}

// sliceNext slices the next off bytes of r.buf.
// It also increments r.pos by off.
func (d *decoder) sliceNext(off int16) []byte {
	// Just use off as ppos.
	off, d.pos = d.pos, d.pos+off
	return d.buf[off:d.pos]
}

// evenBoundary checks if we are on an uneven word boundary.
// If so, it will skip the next byte, which should be a null.
func (d *decoder) evenBoundary() {
	if d.pos%2 == 1 {
		d.pos++
	}
}

// unmarshal unmarshals the terminfo file from f.
// TODO what is the max entry size mean in terminfo(5)?
func (d *decoder) unmarshal() (err error) {
	s, hl := int16(len(d.buf)), d.h.lenBytes()
	// Add 2 extra for the magic.
	if s < hl+2 {
		return ErrSmallFile
	}
	if littleEndian(0, d.buf) != magic {
		return ErrBadHeader
	}
	// Skip magic.
	d.pos = 2
	if err = d.unmarshalHeader(); err != nil {
		return err
	}
	if s-d.pos < d.h.lenCaps() {
		return ErrSmallFile
	}
	if d.h.excessCaps() {
		return ErrBadHeader
	}
	d.ti = new(Terminfo)
	d.unmarshalNames()
	d.unmarshalBools()
	d.evenBoundary()
	d.unmarshalNumbers()
	if err = d.unmarshalStrings(); err != nil || s <= d.pos {
		return err
	}
	// We have extended capabilities.
	d.evenBoundary()
	if s -= d.pos; s < hl {
		return ErrSmallFile
	}
	if err = d.unmarshalHeader(); err != nil {
		return err
	}
	if d.h.badLenExtOff() {
		return ErrBadHeader
	}
	if s-hl < d.h.lenExtCaps() {
		return ErrSmallFile
	}
	if err = d.setExtNameTable(); err != nil {
		return err
	}
	if err = d.unmarshalExtBools(); err != nil {
		return err
	}
	d.evenBoundary()
	if err = d.unmarshalExtNumbers(); err != nil {
		return err
	}
	return d.unmarshalExtStrings()
}

func (d *decoder) unmarshalNames() {
	d.ti.Names = strings.Split(string(d.sliceNext(d.h[lenNames])), "|")
}

// unmarshalHeader unmarshals the terminfo header.
func (d *decoder) unmarshalHeader() error {
	hbuf := d.sliceNext(d.h.lenBytes())
	for i := 0; i < len(d.h); i++ {
		n := littleEndian(int16(i*2), hbuf)
		if n < 0 {
			return ErrBadHeader
		}
		d.h[i] = n
	}
	return nil
}

// unmarshalBools unmarshals the boolean section.
func (d *decoder) unmarshalBools() {
	for i, b := range d.sliceNext(d.h[lenBools]) {
		if b == 1 {
			d.ti.Bools[i] = true
		}
	}
}

// unmarshalNumbers unmarshals the numeric section.
func (d *decoder) unmarshalNumbers() {
	nbuf := d.sliceNext(d.h[lenNumbers] * 2)
	for i := int16(0); i < d.h[lenNumbers]; i++ {
		if n := littleEndian(i*2, nbuf); n > -1 {
			d.ti.Numbers[i] = n
		}
	}
}

// unmarshalStrings unmarshals the string and string table sections.
func (d *decoder) unmarshalStrings() error {
	sbuf := d.sliceNext(d.h[lenStrings] * 2)
	table := d.sliceNext(d.h[lenTable])
	for i := int16(0); i < d.h[lenStrings]; i++ {
		if off := littleEndian(i*2, sbuf); off > -1 {
			end := indexNull(off, table)
			if end == -1 {
				return ErrBadString
			}
			d.ti.Strings[i] = string(table[off:end])
		}
	}
	return nil
}

// setExtNameTable splits the string table into a string table and a name table.
// This allows us to unmarshal the capabilities and their names concurrently.
func (d *decoder) setExtNameTable() error {
	d.extNameOffPos = d.pos + d.h.extNameOffsOff()
	lenExtNameOffs := d.h.lenExtNameOffs()
	// Find last string offset.
	vpos := d.extNameOffPos
	var voff int16
	for {
		vpos -= 2
		if vpos < d.pos {
			return ErrBadString
		}
		d.h[lenExtStrings]--
		if voff = littleEndian(vpos, d.buf); voff > -1 {
			break
		}
	}
	// Unmarshal the capability value.
	d.extStringTable = d.buf[d.extNameOffPos+lenExtNameOffs:]
	vend := indexNull(voff, d.extStringTable)
	if vend == -1 {
		return ErrBadString
	}
	// The rest is the name table
	d.extNameTable = d.extStringTable[vend+1:]
	// Unmarshal the capability name.
	koff := littleEndian(vpos+lenExtNameOffs, d.buf)
	kend := indexNull(koff, d.extNameTable)
	if kend == -1 {
		return ErrBadString
	}
	// Now set them in the map, then truncate extStringTable and extNameTable to not include them.
	d.ti.ExtStrings = make(map[string]string)
	d.ti.ExtStrings[string(d.extNameTable[koff:kend])] = string(d.extStringTable[voff:vend])
	d.extStringTable = d.extStringTable[:voff]
	d.extNameTable = d.extNameTable[:koff]
	return nil
}

// nextExtName returns the offset and ending of the next capability name.
func (d *decoder) nextExtName() (off, end int16) {
	off = littleEndian(d.extNameOffPos, d.buf)
	d.extNameOffPos += 2
	end = indexNull(off, d.extNameTable)
	return
}

// unmarshalExtBools unmarshals the extended boolean section.
func (d *decoder) unmarshalExtBools() error {
	d.ti.ExtBools = make(map[string]bool)
	for _, b := range d.sliceNext(d.h[lenExtBools]) {
		off, end := d.nextExtName()
		if end == -1 {
			return ErrBadString
		}
		if b == 1 {
			d.ti.ExtBools[string(d.extNameTable[off:end])] = true
		}
	}
	return nil
}

// unmarshalExtNumbers unmarshals the extended numeric section.
func (d *decoder) unmarshalExtNumbers() error {
	d.ti.ExtNumbers = make(map[string]int16)
	nbuf := d.sliceNext(d.h[lenExtNumbers] * 2)
	for i := int16(0); i < d.h[lenExtNumbers]; i++ {
		off, end := d.nextExtName()
		if end == -1 {
			return ErrBadString
		}
		if n := littleEndian(i*2, nbuf); n > -1 {
			d.ti.ExtNumbers[string(d.extNameTable[off:end])] = n
		}
	}
	return nil
}

// unmarshalExtStrings unmarshals the extended string and string table sections.
func (d *decoder) unmarshalExtStrings() error {
	// lpos is the last position.
	for lpos := d.pos + d.h[lenExtStrings]*2; d.pos < lpos; d.pos += 2 {
		koff, kend := d.nextExtName()
		if kend == -1 {
			return ErrBadString
		}
		if voff := littleEndian(d.pos, d.buf); voff > -1 {
			vend := indexNull(voff, d.extStringTable)
			if vend == -1 {
				return ErrBadString
			}
			d.ti.ExtStrings[string(d.extNameTable[koff:kend])] = string(d.extStringTable[voff:vend])
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

// excessCaps returns true if there are too many capabilities and false otherwise.
func (h header) excessCaps() bool {
	if h[lenBools] > caps.BoolCount ||
		h[lenNumbers] > caps.NumberCount ||
		h[lenStrings] > caps.StringCount {
		return true
	}
	return false
}

// badLenExtOff returns true if the length of the offsets is wrong and false otherwise.
// The length of the offsets must be equal to the total number of capabilities (the name offsets)
// and strings (the string offsets).
func (h header) badLenExtOff() bool {
	return h[lenExtBools]+h[lenExtNumbers]+h[lenExtStrings]*2 != h[lenExtOff]
}

// extNameOffsOff returns the offset from where the name offsets begin.
func (h header) extNameOffsOff() int16 {
	// The following works because
	// r.h[lenExtOff] == r.h[lenExtBools]+r.h[lenExtNumbers]+r.h[lenExtStrings]*2.
	// See the check in r.unmarshal.
	return h[lenExtBools]%2 +
		h[lenExtNumbers] +
		h[lenExtOff]
}

// lenExtNameOffs returns the length of the name offsets.
func (h header) lenExtNameOffs() int16 {
	return (h[lenExtOff] - h[lenExtStrings]) * 2
}
