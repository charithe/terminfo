package terminfo

type stack []interface{}

func (st *stack) push(v interface{}) {
	*st = append(*st, v)
}

func (st *stack) pop() (v interface{}) {
	if len(*st) == 0 {
		return nil
	}
	v = (*st)[len(*st)-1]
	*st = (*st)[:len(*st)-1]
	return
}

func (st *stack) popInt() int {
	if ai, ok := st.pop().(int); ok {
		return ai
	}
	return 0
}

func (st *stack) popTwoInt() (bi int, ai int) {
	bi = st.popInt()
	ai = st.popInt()
	return
}

func (st *stack) popBool() bool {
	if ab, ok := st.pop().(bool); ok {
		return ab
	}
	return false
}

func (st *stack) popByte() byte {
	if ab, ok := st.pop().(byte); ok {
		return ab
	}
	return 0
}

func (st *stack) popString() string {
	if as, ok := st.pop().(string); ok {
		return as
	}
	return ""
}
