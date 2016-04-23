package terminfo

import (
	"errors"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/nhooyr/terminfo/caps"
)

// Package errors.
var (
	ErrSmallFile = errors.New("terminfo: file too small")
	ErrBadMagic  = errors.New("terminfo: wrong filetype for terminfo file")
	ErrEmptyTerm = errors.New("terminfo: empty term name")
)

// Terminfo describes a terminal's capabilities.
type Terminfo struct {
	Names       []string
	BoolCaps    [caps.BoolCount]bool
	NumericCaps [caps.NumericCount]int16
	StringCaps  [caps.StringCount]string
}

// OpenEnv calls Open with the name as $TERM.
func OpenEnv() (ti *Terminfo, err error) {
	return Open(os.Getenv("TERM"))
}

// Open follows the behavior described in terminfo(5) to find correct the terminfo file
// using the name and then return a Terminfo struct that describes the file.
func Open(name string) (ti *Terminfo, err error) {
	if name == "" {
		return nil, ErrEmptyTerm
	} else if terminfo := os.Getenv("TERMINFO"); terminfo != "" {
		return openDir(terminfo, name)
	} else if home := os.Getenv("HOME"); home != "" {
		ti, err = openDir(home+"/.terminfo", name)
		if err == nil {
			return
		}
	}
	if dirs := os.Getenv("TERMINFO_DIRS"); dirs != "" {
		for _, dir := range strings.Split(dirs, ":") {
			if dir == "" {
				dir = "/usr/share/terminfo"
			}
			ti, err = openDir(dir, name)
			if err == nil {
				return
			}
		}
	}
	return openDir("/usr/share/terminfo", name)
}

// openDir reads the Terminfo file specified by the dir and name.
func openDir(dir, name string) (ti *Terminfo, err error) {
	// Try typical *nix path.
	b, err := ioutil.ReadFile(dir + "/" + name[0:1] + "/" + name)
	if err == nil {
		return readTerminfo(b)
	}
	// Fallback to the darwin specific path.
	b, err = ioutil.ReadFile(dir + "/" + strconv.FormatUint(uint64(name[0]), 16) + "/" + name)
	if err != nil {
		return
	}
	return readTerminfo(b)
}

// readTerminfo reads the Terminfo file in buf into a Terminfo struct and returns it.
// TODO extended reader
// TODO break this function up
func readTerminfo(buf []byte) (*Terminfo, error) {
	if len(buf) < 6 {
		return nil, ErrSmallFile
	}
	// Read the header.
	var h header
	for i := 0; i < len(h); i++ {
		// TODO The value -1 is represented by the two bytes 0377, 0377; other negative values are illegal.
		// I think this applies to all short integers. But I will wait for a email reply on the ncurses mailing list for advice on how to handle this.
		h[i] = littleEndian(i*2, buf)
	}
	if int(h.lenFile()) > len(buf) {
		return nil, ErrSmallFile
	} else if h.badMagic() {
		return nil, ErrBadMagic
	}

	// Read name section.
	pi := h.len()
	i := pi + h.lenNames()
	ti := new(Terminfo)
	ti.Names = strings.Split(string(buf[pi:i]), "|")

	// Read the boolean section.
	pi, i = i, i+h.lenBools()
	for i, b := range buf[pi:i] {
		if b == 1 {
			ti.BoolCaps[i] = true
		}
	}
	if h.skipNull() {
		// Skip extra null byte inserted to align everything on word boundaries.
		i++
	}

	// Read the numeric section.
	pi, i = i, i+h.lenNumeric()
	nbuf := buf[pi:i]
	for j := 0; j < len(nbuf); j += 2 {
		if n := littleEndian(j, nbuf); n > -1 {
			ti.NumericCaps[j/2] = n
		}
	}

	// Read the string and string table section.
	// TODO panic if no ending character fix dis shit
	pi, i = i, i+h.lenStrings()
	sbuf := buf[pi:i]
	table := buf[i : i+h.lenTable()]
	for j := 0; j < len(sbuf); j += 2 {
		if off := littleEndian(j, sbuf); off > -1 {
			x := off
			for ; table[x] != 0; x++ {
			}
			ti.StringCaps[j/2] = string(table[off:x])
		}
	}

	return ti, nil
}

// Parm evaluates a terminfo parameterized string, such as caps.SetAForeground,
// and returns the result.
func (ti *Terminfo) Parm(s string, p ...int) string {
	pz := getParametizer(s)
	defer pz.free()
	// make sure we always have 9 parameters -- makes it easier
	// later to skip checks
	for i := 0; i < len(pz.params) && i < len(p); i++ {
		pz.params[i] = p[i]
	}
	return pz.run()
}
