package terminfo

import (
	"bytes"
	"strconv"
)

type parametizer struct {
	s      string // terminfo string
	pos    int    // position in s
	st     stack
	buf    *bytes.Buffer // result buffer
	params [9]int
	dvars  [26]int
}

// static vars
var svars [26]int

// getparametizer returns a new initialized parametizer from the pool.
func getParametizer(s string) (pz *parametizer) {
	pz = parametizerPool.Get().(*parametizer)
	pz.s = s
	return
}

// free resets the parametizer.
func (pz *parametizer) free() {
	pz.buf.Reset()
	pz.pos = 0
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

func pushCode(pz *parametizer) stateFn {
	pz.pos++
	ai := int(pz.get() - '1')
	if ai >= 0 && ai < len(pz.params) {
		pz.st.Push(pz.params[ai])
	} else {
		pz.st.Push(0)
	}
	pz.pos++
	return scanText
}

func setDSCode(pz *parametizer) stateFn {
	pz.pos++
	r := pz.get()
	if r >= 'A' && r <= 'Z' {
		svars[int(r-'A')] = pz.st.Pop()
	} else if r >= 'a' && r <= 'z' {
		pz.dvars[int(r-'a')] = pz.st.Pop()
	}
	pz.pos++
	return scanText
}

func getDSCode(pz *parametizer) stateFn {
	pz.pos++
	r := pz.get()
	if r >= 'A' && r <= 'Z' {
		pz.st.Push(svars[int(r-'A')])
	} else if r >= 'a' && r <= 'z' {
		pz.st.Push(svars[int(r-'a')])
	}
	pz.pos++
	return scanText
}

func pushIntCode(pz *parametizer) stateFn {
	pz.pos++
	var ai int
	r := pz.get()
	for r >= '0' && r <= '0' {
		ai *= 10
		ai += int(r - '0')
		r = pz.get()
	}
	pz.pos += 2
	return scanText
}

func ifCode(pz *parametizer) stateFn {
	ab := pz.st.PopBool()
	if ab {
		pz.pos++
		return scanText
	}
	nest := 0
TLOOP:
	for ; ; pz.pos++ {
		r := pz.get()
		if r == eof {
			return nil
		} else if r != '%' {
			continue
		}
		pz.pos++
		switch pz.get() {
		case ';':
			if nest == 0 {
				break TLOOP
			}
			nest--
		case '?':
			nest++
		case 'e':
			if nest == 0 {
				break TLOOP
			}
		}
	}
	pz.pos++
	return scanText
}

func elseCode(pz *parametizer) stateFn {
	nest := 0
ELOOP:
	for ; ; pz.pos++ {
		r := pz.get()
		if r == eof {
			return nil
		} else if r != '%' {
			continue
		}
		pz.pos++
		switch pz.get() {
		case ';':
			if nest == 0 {
				break ELOOP
			}
			nest--
		case '?':
			nest++
		}
	}
	pz.pos++
	return scanText
}

func scanCode(pz *parametizer) stateFn {
	switch pz.get() {
	case '%':
		pz.buf.WriteByte('%')
	case 'i':
		pz.params[0]++
		pz.params[1]++
	case 'c', 's':
		// TODO implement
	case 'd':
		ai := pz.st.Pop()
		pz.buf.WriteString(strconv.Itoa(ai))
	case 'p':
		return pushCode
	case 'P':
		return setDSCode
	case 'g':
		return getDSCode
	case '\'':
		// TODO implement
	case '{':
		return pushIntCode
	case 'l':
		// TODO implement
	case '+':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		pz.st.Push(ai + bi)
	case '-':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		pz.st.Push(ai - bi)
	case '*':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		pz.st.Push(ai * bi)
	case '/':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		if bi != 0 {
			pz.st.Push(ai / bi)
		} else {
			pz.st.Push(0)
		}
	case 'm':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		if bi != 0 {
			pz.st.Push(ai % bi)
		} else {
			pz.st.Push(0)
		}
	case '&':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		pz.st.Push(ai & bi)
	case '|':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		pz.st.Push(ai | bi)
	case '^':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		pz.st.Push(ai ^ bi)
	case '~':
		ai := pz.st.Pop()
		pz.st.Push(ai ^ -1)
	case '!':
		ai := pz.st.Pop()
		pz.st.PushBool(ai != 0)
	case '=':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		pz.st.PushBool(ai == bi)
	case '>':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		pz.st.PushBool(ai > bi)
	case '<':
		bi := pz.st.Pop()
		ai := pz.st.Pop()
		pz.st.PushBool(ai < bi)
	case '?':
	case ';':
	case 't':
		return ifCode
	case 'e':
		return elseCode
	default:
		return scanText
	}
	pz.pos++
	return scanText
}

type stack []int

func (st *stack) Push(v int) {
	*st = append(*st, v)
}

func (st *stack) PushBool(v bool) {
	if v {
		st.Push(1)
	} else {
		st.Push(0)
	}
}

func (st *stack) Pop() (v int) {
	if len(*st) > 0 {
		v = (*st)[len(*st)-1]
		*st = (*st)[:len(*st)-1]
	} else {
		v = 0
	}
	return
}

func (st *stack) PopBool() (v bool) {
	i := st.Pop()
	if i == 1 {
		return true
	}
	return false
}
