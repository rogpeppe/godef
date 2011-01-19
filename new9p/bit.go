package plan9

func gbit8(b []byte) (uint8, []byte) {
	return uint8(b[0]), b[1:]
}

func gbool(b []byte) (bool, []byte) {
	return b[0] != 0, b[1:]
}

func gbit16(b []byte) (uint16, []byte) {
	return uint16(b[0]) | uint16(b[1])<<8, b[2:]
}

func gbit32(b []byte) (uint32, []byte) {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24, b[4:]
}

func gbit64(b []byte) (uint64, []byte) {
	lo, b := gbit32(b)
	hi, b := gbit32(b)
	return uint64(hi)<<32 | uint64(lo), b
}

func gstring(b []byte) (string, []byte) {
	n, b := gbit16(b)
	return string(b[0:n]), b[n:]
}

func pbool(b []byte, x bool) []byte {
	if x {
		return append(b, 1)
	}
	return append(b, 0)
}

func pbit8(b []byte, x uint8) []byte {
	return append(b, x)
}

func pbit16(b []byte, x uint16) []byte {
	return append(b, byte(x), byte(x>>8))
}

func pbit32(b []byte, x uint32) []byte {
	return append(b, byte(x), byte(x>>8), byte(x>>16), byte(x>>24))
}

func pbit64(b []byte, x uint64) []byte {
	b = pbit32(b, uint32(x))
	b = pbit32(b, uint32(x>>32))
	return b
}

func pstring(b []byte, s string) []byte {
	if len(s) >= 1<<16 {
		panic(ProtocolError("string too long"))
	}
	b = pbit16(b, uint16(len(s)))
	b = append(b, []byte(s)...)
	return b
}
