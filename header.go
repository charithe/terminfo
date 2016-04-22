package terminfo

type header [6]int16

// badMagic returns false if the correct magic number is set on the header and true otherwise.
func (h header) badMagic() bool {
	if h[0] == 0x11A {
		return false
	}
	return true
}

// lenNames returns the length of name section in bytes
func (h header) lenNames() int16 {
	return h[1]
}

// lenBools returns the length of boolean section in bytes
func (h header) lenBools() int16 {
	return h[2]
}

// lenNumeric returns the length of numeric section in int16
func (h header) lenNumeric() int16 {
	return h[3]
}

// lenStrings returns the length of string section in int16
func (h header) lenStrings() int16 {
	return h[4]
}

// lenTable returns the length of string table in bytes.
func (h header) lenTable() int16 {
	return h[5]
}

// needAlignment checks if a null byte is needed to align everything on word boundaries.
func (h header) needAlignment() bool {
	return (h.lenNames()+h.lenBools())%2 == 1
}
