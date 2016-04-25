package terminfo

import (
	"bytes"
	"io"
	"strconv"
	"sync"
)

// parametizer represents the scanners state.
type parametizer struct {
	s        string          // terminfo string
	pos      int             // position in s
	nest     int             // nesting level of if statements
	st       stack           // terminfo var stack
	skipElse bool            // see skipText.
	buf      *bytes.Buffer   // result buffer
	params   [9]interface{}  // paramters
	dvars    [26]interface{} // dynamic vars
}

// Parm evaluates a terminfo parameterized string, such as cap.SetAForeground,
// and returns the result.
func Parm(s string, p ...interface{}) string {
	pz := getParametizer(s)
	defer pz.free()
	// make sure we always have 9 parameters -- makes it easier
	// later to skip checks and its faster
	for i := 0; i < len(pz.params) && i < len(p); i++ {
		pz.params[i] = p[i]
	}
	return pz.run()
}

// static vars
var svars [26]interface{}

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
	pz.params = [9]interface{}{}
	pz.dvars = [26]interface{}{}
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
	for {
		ch, err := pz.get()
		if err != nil {
			pz.writeFrom(ppos)
			return nil
		}
		if ch == '%' {
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
		pz.params[0] = pz.params[0].(int) + 1
		pz.params[1] = pz.params[1].(int) + 1
	case 'c':
		pz.buf.WriteByte(pz.st.popByte())
	case 's':
		pz.buf.WriteString(pz.st.popString())
	case 'd':
		pz.buf.WriteString(strconv.Itoa(pz.st.popInt()))
	case ':':
		// TODO implement
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
		pz.st.push(ch)
	case '{':
		pz.pos++
		return pushInt
	case 'l':
		pz.st.push(len(pz.st.popString()))
	case '+':
		bi, ai := pz.st.popTwoInt()
		pz.st.push(ai + bi)
	case '-':
		bi, ai := pz.st.popTwoInt()
		pz.st.push(ai - bi)
	case '*':
		bi, ai := pz.st.popTwoInt()
		pz.st.push(ai * bi)
	case '/':
		bi, ai := pz.st.popTwoInt()
		if bi != 0 {
			pz.st.push(ai / bi)
		} else {
			pz.st.push(0)
		}
	case 'm':
		bi, ai := pz.st.popTwoInt()
		if bi != 0 {
			pz.st.push(ai % bi)
		} else {
			pz.st.push(0)
		}
	case '&':
		bi, ai := pz.st.popTwoInt()
		pz.st.push(ai & bi)
	case '|':
		bi, ai := pz.st.popTwoInt()
		pz.st.push(ai | bi)
	case '^':
		bi, ai := pz.st.popTwoInt()
		pz.st.push(ai ^ bi)
	case '~':
		pz.st.push(pz.st.popInt() ^ -1)
	case '!':
		pz.st.push(pz.st.popInt() != 0)
	case '=':
		bi, ai := pz.st.popTwoInt()
		pz.st.push(ai == bi)
	case '>':
		bi, ai := pz.st.popTwoInt()
		pz.st.push(ai > bi)
	case '<':
		bi, ai := pz.st.popTwoInt()
		pz.st.push(ai < bi)
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
		pz.st.push(pz.params[ai])
	} else {
		pz.st.push(0)
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
		svars[int(ch-'A')] = pz.st.pop()
	} else if ch >= 'a' && ch <= 'z' {
		pz.dvars[int(ch-'a')] = pz.st.pop()
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
		pz.st.push(svars[int(ch-'A')])
	} else if ch >= 'a' && ch <= 'z' {
		pz.st.push(svars[int(ch-'a')])
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
	pz.st.push(ai)
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
	if err != nil {
		return nil
	}
	for pz.pos++; ch != '%'; pz.pos++{
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
