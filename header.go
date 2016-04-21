package terminfo

type header [6]int16

// Named indexes of header
const (
	lenNames   = iota + 1 // length of names section in bytes
	lenBool               // length of boolean section in bytes
	lenNumeric            // length of numeric section in int16
	lenStrings            // length of offset section in int166
	lenTable              // length of string table in bytes
)
