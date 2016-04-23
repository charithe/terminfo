package terminfo

// littleEndian decodes a int16 starting at i in buf using little-endian byte order.
func littleEndian(i int, buf []byte) int16 {
	return int16(buf[i+1])<<8 | int16(buf[i])
}
