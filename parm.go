package terminfo

import (
	"bytes"
	"io"
	"strconv"
	"sync"
)

type stack []interface{}

func (stk *stack) push(v interface{}) {
	*stk = append(*stk, v)
}

func (stk *stack) pop() interface{} {
	if len(*stk) == 0 {
		return nil
	}
	v := (*stk)[len(*stk)-1]
	*stk = (*stk)[:len(*stk)-1]
	return v
}

func (stk *stack) popInt() int {
	if ai, ok := stk.pop().(int); ok {
		return ai
	}
	return 0
}

func (stk *stack) popBool() bool {
	if ab, ok := stk.pop().(bool); ok {
		return ab
	}
	return false
}

func (stk *stack) popByte() byte {
	if ab, ok := stk.pop().(byte); ok {
		return ab
	}
	return 0
}

func (stk *stack) popString() string {
	if as, ok := stk.pop().(string); ok {
		return as
	}
	return ""
}

// parametizer represents the scanners state.
type parametizer struct {
	s        string          // terminfo string
	pos      int             // position in s
	nest     int             // nesting level of if statements
	stk      stack           // terminfo var stack
	skipElse bool            // controls which fuction skipText returns
	buf      *bytes.Buffer   // result buffer
	params   [9]interface{}  // paramters
	dvars    [26]interface{} // dynamic vars
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
func getParametizer(s string) *parametizer {
	pz := parametizerPool.Get().(*parametizer)
	pz.s = s
	return pz
}

// free resets the parametizer.
func (pz *parametizer) free() {
	pz.pos = 0
	pz.nest = 0
	pz.stk = pz.stk[:0]
	pz.buf.Reset()
	pz.params = [9]interface{}{}
	pz.dvars = [26]interface{}{}
	parametizerPool.Put(pz)
}

// Parm evaluates a terminfo parameterized string, such as caps.SetAForeground,
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
		pz.buf.WriteByte(pz.stk.popByte())
	case 's':
		pz.buf.WriteString(pz.stk.popString())
	case 'd':
		pz.buf.WriteString(strconv.Itoa(pz.stk.popInt()))
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
		pz.stk.push(ch)
		// skip the '\''
		pz.pos++
	case '{':
		pz.pos++
		return pushInt
	case 'l':
		pz.stk.push(len(pz.stk.popString()))
	case '+':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		pz.stk.push(ai + bi)
	case '-':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		pz.stk.push(ai - bi)
	case '*':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		pz.stk.push(ai * bi)
	case '/':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		if bi != 0 {
			pz.stk.push(ai / bi)
		} else {
			pz.stk.push(0)
		}
	case 'm':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		if bi != 0 {
			pz.stk.push(ai % bi)
		} else {
			pz.stk.push(0)
		}
	case '&':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		pz.stk.push(ai & bi)
	case '|':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		pz.stk.push(ai | bi)
	case '^':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		pz.stk.push(ai ^ bi)
	case '~':
		pz.stk.push(pz.stk.popInt() ^ -1)
	case '!':
		pz.stk.push(pz.stk.popInt() != 0)
	case '=':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		pz.stk.push(ai == bi)
	case '>':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		pz.stk.push(ai > bi)
	case '<':
		bi, ai := pz.stk.popInt(), pz.stk.popInt()
		pz.stk.push(ai < bi)
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
		pz.stk.push(pz.params[ai])
	} else {
		pz.stk.push(0)
	}
	// skip the '}'
	pz.pos++
	return scanText
}

func setDSVar(pz *parametizer) stateFn {
	ch, err := pz.get()
	if err != nil {
		return nil
	}
	if ch >= 'A' && ch <= 'Z' {
		svars[int(ch-'A')] = pz.stk.pop()
	} else if ch >= 'a' && ch <= 'z' {
		pz.dvars[int(ch-'a')] = pz.stk.pop()
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
		pz.stk.push(svars[int(ch-'A')])
	} else if ch >= 'a' && ch <= 'z' {
		pz.stk.push(svars[int(ch-'a')])
	}
	pz.pos++
	return scanText
}

func pushInt(pz *parametizer) stateFn {
	var ai int
	for {
		ch, err := pz.get()
		if err != nil {
			return nil
		}
		pz.pos++
		if ch < '0' || ch > '9' {
			pz.stk.push(ai)
			return scanText
		}
		ai = (ai * 10) + int(ch-'0')
	}
}

func scanThen(pz *parametizer) stateFn {
	pz.pos++
	if pz.stk.popBool() {
		return scanText
	}
	pz.skipElse = false
	return skipText
}

func skipText(pz *parametizer) stateFn {
	for {
		ch, err := pz.get()
		if err != nil {
			return nil
		}
		pz.pos++
		if ch == '%' {
			break
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
