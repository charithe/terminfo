package terminfo

type stack []int

func (st *stack) PushInt(v int) {
	*st = append(*st, v)
}

func (st *stack) PushBool(v bool) {
	if v {
		st.PushInt(1)
	} else {
		st.PushInt(0)
	}
}

func (st *stack) PushByte(v rune) {
	st.PushInt(int(v))
}

func (st *stack) PopInt() (ai int) {
	if len(*st) == 0 {
		return 0
	}
	ai = (*st)[len(*st)-1]
	*st = (*st)[:len(*st)-1]
	return
}

func (st *stack) PopTwoInt() (bi int, ai int) {
	bi = st.PopInt()
	ai = st.PopInt()
	return
}

func (st *stack) PopBool() (ab bool) {
	if st.PopInt() == 1 {
		return true
	}
	return false
}

func (st *stack) PopByte() (ar rune) {
	return rune(st.PopInt())
}
