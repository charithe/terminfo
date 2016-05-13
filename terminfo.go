// Package terminfo implements terminfo reading in pure go.
package terminfo

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/nhooyr/terminfo/caps"
)

// Terminfo describes a terminal's capabilities.
type Terminfo struct {
	Names      []string
	Bools      [caps.BoolCount]bool
	Numbers    [caps.NumberCount]int16
	Strings    [caps.StringCount]string
	ExtBools   map[string]bool
	ExtNumbers map[string]int16
	ExtStrings map[string]string
}

// Terminfo cache.
var (
	db      = make(map[string]*Terminfo)
	dbMutex = new(sync.RWMutex)
)

// LoadEnv calls Load with the name as $TERM.
func LoadEnv() (*Terminfo, error) {
	return Load(os.Getenv("TERM"))
}

// Returned when no name is provided to Load.
var ErrEmptyTerm = errors.New("terminfo: empty term name")

// Load follows the behavior described in terminfo(5) to find correct the terminfo file
// using the name, reads the file and then returns a Terminfo struct that describes the file.
func Load(name string) (ti *Terminfo, err error) {
	if name == "" {
		return nil, ErrEmptyTerm
	}
	dbMutex.RLock()
	ti, ok := db[name]
	dbMutex.RUnlock()
	if ok {
		return
	}
	if terminfo := os.Getenv("TERMINFO"); terminfo != "" {
		return openDir(terminfo, name)
	}
	if home := os.Getenv("HOME"); home != "" {
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
func openDir(dir, name string) (*Terminfo, error) {
	// Try typical *nix path.
	b, err := ioutil.ReadFile(dir + "/" + name[0:1] + "/" + name)
	if err != nil {
		// Fallback to the darwin specific path.
		b, err = ioutil.ReadFile(dir + "/" + strconv.FormatUint(uint64(name[0]), 16) + "/" + name)
		if err != nil {
			return nil, err
		}
	}
	r := &decoder{buf: b}
	if err = r.unmarshal(); err != nil {
		return nil, err
	}
	// Cache the Terminfo struct.
	dbMutex.Lock()
	for _, n := range r.ti.Names {
		db[n] = r.ti
	}
	dbMutex.Unlock()
	return r.ti, nil
}

// Color takes a foreground and background color and returns string
// that sets them for this terminal.
func (ti *Terminfo) Color(fg, bg int) (rv string) {
	maxColors := int(ti.Numbers[caps.MaxColors])
	// Map bright colors to lower versions if color table only holds 8.
	if maxColors == 8 {
		if fg > 7 && fg < 16 {
			fg -= 8
		}
		if bg > 7 && bg < 16 {
			bg -= 8
		}
	}
	if maxColors > fg && fg >= 0 {
		rv += ti.Parm(caps.SetAForeground, fg)
	}
	if maxColors > bg && bg >= 0 {
		rv += ti.Parm(caps.SetABackground, bg)
	}
	return
}

// Parm calls the function Parm with the string in ti.Strings at
// i and the variadic arguments.
func (ti *Terminfo) Parm(i int, p ...interface{}) string {
	return Parm(ti.Strings[i], p...)
}

// Puts emits the string to the writer, but expands inline padding
// indications (of the form $<[delay]> where [delay] is msec) to
// a suitable number of padding characters (usually null bytes) based
// upon the supplied baud.  At high baud rates, more padding characters
// will be inserted.
func (ti *Terminfo) Puts(w io.Writer, s string, lines, baud int) {
	for {
		start := strings.Index(s, "$<")
		if start == -1 {
			// Most strings don't need padding, which is good news!
			io.WriteString(w, s)
			return
		}
		io.WriteString(w, s[:start])
		s = s[start+2:]
		end := strings.Index(s, ">")
		if end == -1 {
			// Unterminated... just emit bytes unadulterated.
			io.WriteString(w, "$<"+s)
			return
		}
		val := s[:end]
		s = s[end+1:]
		var ms int
		var dot, mandatory, asterisk bool
		unit := 1000
		for _, ch := range val {
			if ch >= '0' && ch <= '9' {
				ms = (ms * 10) + int(ch-'0')
				if dot {
					unit *= 10
				}
			} else if ch == '.' && !dot {
				dot = true
			} else if ch == '*' && !asterisk {
				ms *= lines
				asterisk = true
			} else if ch == '/' {
				mandatory = true
			} else {
				break
			}
		}
		n := ((baud / 8) / unit) * ms
		pad := ti.Strings[caps.PadChar]
		b := make([]byte, len(pad)*n)
		for bp := copy(b, pad); bp < len(b); bp *= 2 {
			copy(b[bp:], b[:bp])
		}
		if (!ti.Bools[caps.XonXoff] && baud > int(ti.Numbers[caps.PaddingBaudRate])) || mandatory {
			w.Write(b)
		}
	}
}

// Goto returns a string suitable for addressing the cursor at the given
// row and column. The origin 0, 0 is in the upper left corner of the screen.
func (ti *Terminfo) Goto(row, col int) string {
	return ti.Parm(caps.CursorAddress, row, col)
}
