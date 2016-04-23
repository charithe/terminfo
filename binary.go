package terminfo

func littleEndian(i int, buf []byte) int16 {
	return int16(buf[i+1])<<8 | int16(buf[i])
}
