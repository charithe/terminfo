package terminfo

type stack []int

func (st *stack) pushInt(v int) {
	*st = append(*st, v)
}

func (st *stack) pushBool(v bool) {
	if v {
		st.pushInt(1)
	} else {
		st.pushInt(0)
	}
}

func (st *stack) pushByte(v byte) {
	st.pushInt(int(v))
}

func (st *stack) popInt() (ai int) {
	if len(*st) == 0 {
		return 0
	}
	ai = (*st)[len(*st)-1]
	*st = (*st)[:len(*st)-1]
	return
}

func (st *stack) popTwoInt() (bi int, ai int) {
	bi = st.popInt()
	ai = st.popInt()
	return
}

func (st *stack) popBool() (ab bool) {
	if st.popInt() == 1 {
		return true
	}
	return false
}

func (st *stack) popByte() (ar byte) {
	return byte(st.popInt())
}
