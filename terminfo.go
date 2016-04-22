package terminfo

import (
	"encoding/binary"
	"errors"
	"io"
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

func openDir(dir string) (ti *Terminfo, err error) {
	name := os.Getenv("TERM")
	if name == "" {
		return nil, errors.New("terminfo: no TERM envirnoment variable set")
	}
	// Try typical *nix path.
	f, err := os.Open(dir + "/" + name[0:1] + "/" + name)
	if err == nil {
		return readTerminfo(f)
	}
	// Fallback to darwin specific path.
	f, err = os.Open(dir + "/" + strconv.FormatUint(uint64(name[0]), 16) + "/" + name)
	return readTerminfo(f)
}

// TODO The value -1 is represented by the two bytes 0377, 0377; other negative values are illegal.
func readTerminfo(r io.ReadSeeker) (ti *Terminfo, err error) {
	// Read the header.
	var h header
	if err = binary.Read(r, binary.LittleEndian, &h); err != nil {
		return nil, err
	}

	if h.badMagic() {
		return nil, errors.New("terminfo: wrong filetype for terminfo file")
	}

	// Read name section.
	names := make([]byte, h.lenNames())
	if _, err = io.ReadFull(r, names); err != nil {
		return nil, err
	}
	ti = new(Terminfo)
	ti.Names = strings.Split(string(names), "|")

	// Read the boolean section.
	bools := make([]byte, h.lenBools())
	if _, err = io.ReadFull(r, bools); err != nil {
		return nil, err
	}
	if h.needAlignment() {
		// An extra null byte was inserted to align everything on word boundaries.
		// Lets skip it.
		r.Seek(1, 1)
	}
	for i, b := range bools {
		if b == 1 {
			ti.BoolCaps[i] = true
		}
	}

	// Read the numeric section.
	numbers := make([]int16, h.lenNumeric())
	if err = binary.Read(r, binary.LittleEndian, numbers); err != nil {
		return nil, err
	}
	for i, n := range numbers {
		if n > -1 {
			ti.NumericCaps[i] = n
		}
	}

	// Read the string section.
	strings := make([]int16, h.lenStrings())
	if err = binary.Read(r, binary.LittleEndian, strings); err != nil {
		return nil, err
	}

	// Read the string table section.
	table := make([]byte, h.lenTable())
	if _, err = io.ReadFull(r, table); err != nil {
		return nil, err
	}

	// Read the strings referenced in the string section from the string table.
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
