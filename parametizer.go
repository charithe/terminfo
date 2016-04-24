package terminfo

import (
	"bytes"
	"io"
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
		pz.buf = bytes.NewBuffer(make([]byte, 0, 45))
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
	pz.st = pz.st[:0]
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

// get returns the current byte.
func (pz *parametizer) get() (byte, error) {
	if pz.pos >= len(pz.s) {
		return 0, io.EOF
	}
	return pz.s[pz.pos], nil
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
		ch, err := pz.get()
		if err != nil {
			pz.writeFrom(ppos)
			return nil
		} else if ch == '%' {
			pz.writeFrom(ppos)
			pz.pos++
			return scanCode
		}
		pz.pos++
	}
}

func scanCode(pz *parametizer) stateFn {
	ch, err := pz.get()
	if err != nil {
		return nil
	}
	switch ch {
	case '%':
		pz.buf.WriteByte('%')
	case 'i':
		pz.params[0]++
		pz.params[1]++
	case 'c':
		pz.buf.WriteByte(pz.st.popByte())
	case 's':
		// no one uses this
	case 'd':
		pz.buf.WriteString(strconv.Itoa(pz.st.popInt()))
	case ':':
		// no one uses this
	case 'p':
		pz.pos++
		return pushParam
	case 'P':
		pz.pos++
		return setDSVar
	case 'g':
		pz.pos++
		return getDSVar
	case '\'':
		pz.pos++
		ch, err = pz.get()
		if err != nil {
			return nil
		}
		pz.st.pushByte(ch)
	case '{':
		pz.pos++
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
	}
	pz.pos++
	return scanText
}

func pushParam(pz *parametizer) stateFn {
	ch, err := pz.get()
	if err != nil {
		return nil
	}
	if ai := int(ch - '1'); ai >= 0 && ai < len(pz.params) {
		pz.st.pushInt(pz.params[ai])
	} else {
		pz.st.pushInt(0)
	}
	pz.pos++
	return scanText
}

func setDSVar(pz *parametizer) stateFn {
	ch, err := pz.get()
	if err != nil {
		return nil
	}
	if ch >= 'A' && ch <= 'Z' {
		svars[int(ch-'A')] = pz.st.popInt()
	} else if ch >= 'a' && ch <= 'z' {
		pz.dvars[int(ch-'a')] = pz.st.popInt()
	}
	pz.pos++
	return scanText
}

func getDSVar(pz *parametizer) stateFn {
	ch, err := pz.get()
	if err != nil {
		return nil
	}
	if ch >= 'A' && ch <= 'Z' {
		pz.st.pushInt(svars[int(ch-'A')])
	} else if ch >= 'a' && ch <= 'z' {
		pz.st.pushInt(svars[int(ch-'a')])
	}
	pz.pos++
	return scanText
}

func pushInt(pz *parametizer) stateFn {
	ch, err := pz.get()
	if err != nil {
		return nil
	}
	var ai int
	for ch >= '0' && ch <= '9' {
		ai = (ai * 10) + int(ch-'0')
		pz.pos++
		ch, err = pz.get()
		if err != nil {
			return nil
		}
	}
	pz.st.pushInt(ai)
	pz.pos++
	return scanText
}

func scanThen(pz *parametizer) stateFn {
	ab := pz.st.popBool()
	pz.pos++
	if ab {
		return scanText
	}
	pz.skipElse = false
	return skipText
}

func skipText(pz *parametizer) stateFn {
	ch, err := pz.get()
	for pz.pos++; ch != '%'; pz.pos++ {
		ch, err = pz.get()
		if err != nil {
			return nil
		}
	}
	if pz.skipElse {
		return skipElse
	}
	return skipThen
}

func skipThen(pz *parametizer) stateFn {
	ch, err := pz.get()
	if err != nil {
		return nil
	}
	pz.pos++
	switch ch {
	case ';':
		if pz.nest == 0 {
			return scanText
		}
		pz.nest--
	case '?':
		pz.nest++
	case 'e':
		if pz.nest == 0 {
			return scanText
		}
	}
	return skipText
}

func skipElse(pz *parametizer) stateFn {
	ch, err := pz.get()
	if err != nil {
		return nil
	}
	pz.pos++
	switch ch {
	case ';':
		if pz.nest == 0 {
			return scanText
		}
		pz.nest--
	case '?':
		pz.nest++
	}
	return skipText
}
