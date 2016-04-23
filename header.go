package terminfo

type header [6]int16

// badMagic returns false if the correct magic number is set on the header and true otherwise.
func (h header) badMagic() bool {
	if h[0] == 0x11A {
		return false
	}
	return true
}

// lenNames returns the length of name section
func (h header) lenNames() int16 {
	return h[1]
}

// lenBools returns the length of boolean section
func (h header) lenBools() int16 {
	return h[2]
}

// lenNumeric returns the length of numeric section
func (h header) lenNumeric() int16 {
	return h[3] * 2 // stored as number of int16
}

// lenStrings returns the length of string section
func (h header) lenStrings() int16 {
	return h[4] * 2 // stored as number of int16
}

// lenTable returns the length of string table in bytes.
func (h header) lenTable() int16 {
	return h[5]
}

func (h header) lenFile() int16 {
	return h[1] + h[2] + h[3] + h[4] + h[5]
}

// extraNull returns true if an extra null byte is needed to align everything
// on word boundaries and false otherwise.
func (h header) isExtraNull() bool {
	return (h.lenNames()+h.lenBools())%2 == 1
}

// len returns the length of the header in bytes.
func (h header) len() int16 {
	return int16(len(h) * 2)
}
