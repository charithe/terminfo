package terminfo

import (
	"errors"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/nhooyr/terminfo/caps"
)

type Terminfo struct {
	Names       []string
	BoolCaps    [caps.BoolCount]bool
	NumericCaps [caps.NumericCount]int16
	StringCaps  [caps.StringCount]string
}

var (
	errSmallFile = errors.New("terminfo: file too small")
	errBadMagic  = errors.New("terminfo: wrong filetype for terminfo file")
)

// Open follows the behavior described in terminfo(5) to find correct the
// terminfo file and then return a Terminfo struct that describes the file.
func Open() (ti *Terminfo, err error) {
	if terminfo := os.Getenv("TERMINFO"); terminfo != "" {
		return openDir(terminfo)
	}
	if home := os.Getenv("HOME"); home != "" {
		ti, err = openDir(home + "/.terminfo")
		if err == nil {
			return
		}
	}
	if dirs := os.Getenv("TERMINFO_DIRS"); dirs != "" {
		for _, dir := range strings.Split(dirs, ":") {
			if dir == "" {
				dir = "/usr/share/terminfo"
			}
			ti, err = openDir(dir)
			if err == nil {
				return
			}
		}
	}
	return openDir("/usr/share/terminfo")
}

var name = os.Getenv("TERM")

func openDir(dir string) (ti *Terminfo, err error) {
	// Try typical *nix path.
	b, err := ioutil.ReadFile(dir + "/" + name[0:1] + "/" + name)
	if err == nil {
		return readTerminfo(b)
	}
	// Fallback to darwin specific path.
	b, err = ioutil.ReadFile(dir + "/" + strconv.FormatUint(uint64(name[0]), 16) + "/" + name)
	if err != nil {
		return
	}
	return readTerminfo(b)
}

// TODO The value -1 is represented by the two bytes 0377, 0377; other negative values are illegal.
func readTerminfo(buf []byte) (*Terminfo, error) {
	if len(buf) < 6 {
		return nil, errSmallFile
	}
	// Read the header.
	var h header
	for i := 0; i < len(h); i++ {
		h[i] = littleEndian(i*2, buf)
	}
	if int(h.lenFile()) > len(buf) {
		return nil, errSmallFile
	} else if h.badMagic() {
		return nil, errBadMagic
	}

	// Read name section.
	pi := h.len()
	i := pi + h.lenNames()
	ti := &Terminfo{Names: strings.Split(string(buf[pi:i]), "|")}

	// Read the boolean section.
	pi, i = i, i+h.lenBools()
	for i, b := range buf[pi:i] {
		if b == 1 {
			ti.BoolCaps[i] = true
		}
	}
	if h.extraNull() {
		// Skip extra null byte inserted to align everything on word boundaries.
		i++
	}

	// Read the numeric section.
	pi, i = i, i+h.lenNumeric()
	nbuf := buf[pi:i]
	for j := 0; j < len(nbuf)-1; j += 2 {
		if n := littleEndian(j, nbuf); n > -1 {
			ti.NumericCaps[j/2] = n
		}
	}

	// Read the string and string table section.
	pi, i = i, i+h.lenStrings()
	sbuf := buf[pi:i]
	table := buf[i : i+h.lenTable()]
	for j := 0; j < len(sbuf)-1; j += 2 {
		if off := littleEndian(j, sbuf); off > -1 {
			x := off
			for ; table[x] != 0; x++ {
			}
			ti.StringCaps[j/2] = string(table[off:x])
		}
	}

	return ti, nil
}

func littleEndian(i int, buf []byte) int16 {
	return int16(buf[i+1])<<8 | int16(buf[i])
}
