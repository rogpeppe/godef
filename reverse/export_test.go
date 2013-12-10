package reverse

func (b *Scanner) SetBufSize(n int) {
	b.buf = make([]byte, n)
}
