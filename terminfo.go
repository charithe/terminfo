package terminfo

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
)

type Terminfo struct {
	BoolCaps    [boolCount]bool
	NumericCaps [numericCount]int16
	StringCaps  [stringCount]string
}

// GetTermInfo follows the behavior described in terminfo(5) as distributed by ncurses
// to find the correct terminfo file.
func GetTermInfo() (ti *Terminfo, err error) {
	if terminfo := os.Getenv("TERMINFO"); terminfo != "" {
		return getTermInfo(terminfo)
	}

	if home := os.Getenv("HOME"); home != "" {
		ti, err = getTermInfo(home + "/.terminfo")
		if err == nil {
			return
		}
	}

	if dirs := os.Getenv("TERMINFO_DIRS"); dirs != "" {
		for _, dir := range strings.Split(dirs, ":") {
			if dir == "" {
				dir = "/usr/share/terminfo"
			}
			ti, err = getTermInfo(dir)
			if err == nil {
				return
			}
		}
	}

	return getTermInfo("/usr/share/terminfo")
}

func getTermInfo(name string) (ti *Terminfo, err error) {
	f, err := readTermInfo(name)
	if err != nil {
		return
	}
	return parseTerminfo(f)
}

func readTermInfo(dir string) (f *os.File, err error) {
	term := os.Getenv("TERM")
	if term == "" {
		return nil, errors.New("terminfo: no TERM envirnoment variable set")
	}

	// first try, the typical *nix path
	terminfo := dir + "/" + term[0:1] + "/" + term
	f, err = os.Open(terminfo)
	if err == nil {
		return
	}

	// fallback to darwin specific dirs structure
	terminfo = dir + "/" + hex.EncodeToString([]byte(term[:1])) + "/" + term
	f, err = os.Open(terminfo)
	return
}

func parseTerminfo(r io.ReadSeeker) (ti *Terminfo, err error) {
	// read in the header
	var h header
	if err = binary.Read(r, binary.LittleEndian, &h); err != nil {
		return nil, err
	}

	ti = new(Terminfo)

	// read name section
	names := make([]byte, h[lenNames])
	if _, err = r.Read(names); err != nil {
		return nil, err
	}

	// read the boolean section
	bools := make([]byte, h[lenBool])
	if _, err = r.Read(bools); err != nil {
		return nil, err
	}
	if (h[lenNames]+h[lenBool])%2 == 1 {
		// old quirk to align everything on word boundaries
		r.Seek(1, 1)
	}
	for i, b := range bools {
		if b == 1 {
			ti.BoolCaps[i] = true
		}
	}

	// read the numeric section
	numbers := make([]int16, h[lenNumeric])
	if err = binary.Read(r, binary.LittleEndian, numbers); err != nil {
		return nil, err
	}
	for i, n := range numbers {
		if n != 0xff && n > -1 {
			ti.NumericCaps[i] = n
		}
	}

	// read the string section
	strings := make([]int16, h[lenStrings])
	if err = binary.Read(r, binary.LittleEndian, strings); err != nil {
		return nil, err
	}

	// read the table section
	table := make([]byte, h[lenTable])
	if _, err = r.Read(table); err != nil {
		return nil, err
	}

	for i, off := range strings {
		if off > -1 {
			j := off
			for ; table[j] != 0; j++ {
			}
			ti.StringCaps[i] = string(table[off:j])
		}
	}

	return ti, nil
}
