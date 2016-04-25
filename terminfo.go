package terminfo

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/nhooyr/terminfo/cap"
)

// Terminfo describes a terminal's capabilities.
type Terminfo struct {
	Names       []string
	BoolCaps    [cap.BoolCount]bool
	NumericCaps [cap.NumericCount]int16
	StringCaps  [cap.StringCount]string
}

// Terminfo cache.
var (
	db      = make(map[string]*Terminfo)
	dbMutex = new(sync.RWMutex)
)

// OpenEnv calls Open with the name as $TERM.
func OpenEnv() (ti *Terminfo, err error) {
	return Open(os.Getenv("TERM"))
}

var ErrEmptyTerm = errors.New("terminfo: empty term name")

// Open follows the behavior described in terminfo(5) to find correct the terminfo file
// using the name and then returns a Terminfo struct that describes the file.
func Open(name string) (ti *Terminfo, err error) {
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
	f, err := os.Open(dir + "/" + name[0:1] + "/" + name)
	if err != nil {
		// Fallback to the darwin specific path.
		f, err = os.Open(dir + "/" + strconv.FormatUint(uint64(name[0]), 16) + "/" + name)
		if err != nil {
			return nil, err
		}
	}
	r := readerPool.Get().(*reader)
	defer r.free()
	if err = r.read(f); err != nil {
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

func (ti *Terminfo) Color(fg, bg int) (rv string) {
	maxColors := int(ti.NumericCaps[cap.MaxColors])
	if maxColors > fg && fg >= 0 {
		rv += Parm(ti.StringCaps[cap.SetAForeground], fg)
	}
	if maxColors > bg && bg >= 0 {
		rv += Parm(ti.StringCaps[cap.SetABackground], bg)
	}
	return
}
