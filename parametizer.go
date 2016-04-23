package terminfo

import (
	"bytes"
	"strconv"
	"sync"
)

// parametizer represents the scanners state.
type parametizer struct {
	s        string        // terminfo string
	pos      int           // position in s
	nest     int           // nesting level of if statements
	st       stack         // terminfo var stack
	skipElse bool          // see skipText.
	buf      *bytes.Buffer // result buffer
	params   [9]int        // paramters
	dvars    [26]int       // dynamic vars
}

// static vars
var svars [26]int

var parametizerPool = sync.Pool{
	New: func() interface{} {
		pz := new(parametizer)
		pz.buf = bytes.NewBuffer(make([]byte, 0, 30))
		return pz
	},
}

// getparametizer returns a new initialized parametizer from the pool.
func getParametizer(s string) (pz *parametizer) {
	pz = parametizerPool.Get().(*parametizer)
	pz.s = s
	return
}

// free resets the parametizer.
func (pz *parametizer) free() {
	pz.pos = 0
	pz.nest = 0
	pz.st = stack{}
	pz.buf.Reset()
	pz.params = [9]int{}
	pz.dvars = [26]int{}
	parametizerPool.Put(pz)
}

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*parametizer) stateFn

func (pz *parametizer) run() string {
	for state := scanText; state != nil; {
		state = state(pz)
	}
	return pz.buf.String()
}

const eof = -1

// get returns the current rune.
func (pz *parametizer) get() rune {
	if pz.pos >= len(pz.s) {
		return eof
	}
	return rune(pz.s[pz.pos])
}

// writeFrom writes the characters from ppos to pos to the buffer.
func (pz *parametizer) writeFrom(ppos int) {
	if pz.pos > ppos {
		// Append remaining characters.
		pz.buf.WriteString(pz.s[ppos:pz.pos])
	}
}

// scanText scans until the next code.
func scanText(pz *parametizer) stateFn {
	ppos := pz.pos
	// Find next verb.
	for {
		switch pz.get() {
		case '%':
			pz.writeFrom(ppos)
			pz.pos++
			return scanCode
		case eof:
			pz.writeFrom(ppos)
			return nil
		}
		pz.pos++
	}
}

func skipText(pz *parametizer) stateFn {
	r := pz.get()
	for pz.pos++; r != '%'; pz.pos++ {
		r = pz.get()
		if r == eof {
			return nil
		}
	}
	if pz.skipElse {
		return skipElse
	}
	return skipThen
}

func scanCode(pz *parametizer) stateFn {
	switch r := pz.get(); r {
	case '%':
		pz.buf.WriteByte('%')
	case 'i':
		pz.params[0]++
		pz.params[1]++
	case 'c':
		pz.buf.WriteRune(pz.st.popRune())
	case 's':
		// no one uses this
	case 'd':
		pz.buf.WriteString(strconv.Itoa(pz.st.popInt()))
	case ':':
		// no one uses this
	case 'p':
		return pushParam
	case 'P':
		return setDSVar
	case 'g':
		return getDSVar
	case '\'':
		pz.pos++
		pz.st.pushRune(pz.get())
	case '{':
		return pushInt
	case 'l':
		pz.st.pushInt(len(strconv.Itoa(pz.st.popInt())))
	case '+':
		bi, ai := pz.st.popTwoInt()
		pz.st.pushInt(ai + bi)
	case '-':
		bi, ai := pz.st.popTwoInt()
		pz.st.pushInt(ai - bi)
	case '*':
		bi, ai := pz.st.popTwoInt()
		pz.st.pushInt(ai * bi)
	case '/':
		bi, ai := pz.st.popTwoInt()
		if bi != 0 {
			pz.st.pushInt(ai / bi)
		} else {
			pz.st.pushInt(0)
		}
	case 'm':
		bi, ai := pz.st.popTwoInt()
		if bi != 0 {
			pz.st.pushInt(ai % bi)
		} else {
			pz.st.pushInt(0)
		}
	case '&':
		bi, ai := pz.st.popTwoInt()
		pz.st.pushInt(ai & bi)
	case '|':
		bi, ai := pz.st.popTwoInt()
		pz.st.pushInt(ai | bi)
	case '^':
		bi, ai := pz.st.popTwoInt()
		pz.st.pushInt(ai ^ bi)
	case '~':
		ai := pz.st.popInt()
		pz.st.pushInt(ai ^ -1)
	case '!':
		ai := pz.st.popInt()
		pz.st.pushBool(ai != 0)
	case '=':
		bi, ai := pz.st.popTwoInt()
		pz.st.pushBool(ai == bi)
	case '>':
		bi, ai := pz.st.popTwoInt()
		pz.st.pushBool(ai > bi)
	case '<':
		bi, ai := pz.st.popTwoInt()
		pz.st.pushBool(ai < bi)
	case '?':
	case ';':
	case 't':
		return scanThen
	case 'e':
		pz.skipElse = true
		return skipText
	default:
		return scanText
	}
	pz.pos++
	return scanText
}

func pushParam(pz *parametizer) stateFn {
	pz.pos++
	ai := int(pz.get() - '1')
	if ai >= 0 && ai < len(pz.params) {
		pz.st.pushInt(pz.params[ai])
	} else {
		pz.st.pushInt(0)
	}
	pz.pos++
	return scanText
}

func setDSVar(pz *parametizer) stateFn {
	pz.pos++
	r := pz.get()
	if r >= 'A' && r <= 'Z' {
		svars[int(r-'A')] = pz.st.popInt()
	} else if r >= 'a' && r <= 'z' {
		pz.dvars[int(r-'a')] = pz.st.popInt()
	}
	pz.pos++
	return scanText
}

func getDSVar(pz *parametizer) stateFn {
	pz.pos++
	r := pz.get()
	if r >= 'A' && r <= 'Z' {
		pz.st.pushInt(svars[int(r-'A')])
	} else if r >= 'a' && r <= 'z' {
		pz.st.pushInt(svars[int(r-'a')])
	}
	pz.pos++
	return scanText
}

func pushInt(pz *parametizer) stateFn {
	pz.pos++
	r, ai := pz.get(), 0
	for r >= '0' && r <= '9' {
		ai = (ai * 10) + int(r-'0')
		pz.pos++
		r = pz.get()
	}
	pz.st.pushInt(ai)
	pz.pos++
	return scanText
}

func scanThen(pz *parametizer) stateFn {
	ab := pz.st.popBool()
	if ab {
		pz.pos++
		return scanText
	}
	pz.pos++
	pz.skipElse = false
	return skipText
}

func skipThen(pz *parametizer) stateFn {
	switch pz.get() {
	case ';':
		if pz.nest == 0 {
			pz.pos++
			return scanText
		}
		pz.nest--
	case '?':
		pz.nest++
	case 'e':
		if pz.nest == 0 {
			pz.pos++
			return scanText
		}
	}
	pz.pos++
	return skipText
}

func skipElse(pz *parametizer) stateFn {
	switch pz.get() {
	case ';':
		if pz.nest == 0 {
			pz.pos++
			return scanText
		}
		pz.nest--
	case '?':
		pz.nest++
	}
	pz.pos++
	return skipText
}
