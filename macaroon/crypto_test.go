package macaroon

import (
	gc "gopkg.in/check.v1"
)

type cryptoSuite struct{}

var _ = gc.Suite(&cryptoSuite{})

func (*cryptoSuite) TestEncDec(c *gc.C) {
	key := []byte("a key")
	text := []byte("some text")
	b, err := encrypt(key, text)
	c.Assert(err, gc.IsNil)
	t, err := decrypt(key, b)
	c.Assert(err, gc.IsNil)
	c.Assert(string(t), gc.Equals, string(text))
}
