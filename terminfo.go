package terminfo

import (
	"errors"
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
	for i := range r.ti.Names {
		db[r.ti.Names[i]] = r.ti
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
		rv += Parm(ti.Strings[caps.SetAForeground], fg)
	}
	if maxColors > bg && bg >= 0 {
		rv += Parm(ti.Strings[caps.SetABackground], bg)
	}
	return
}
