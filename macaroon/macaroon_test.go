package macaroon_test

import (
	"testing"

	gc "gopkg.in/check.v1"
	"code.google.com/p/rog-go/macaroon"
	_ "net/http"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type macaroonSuite struct {}

var _ = gc.Suite(&macaroonSuite{})

func never(string) (bool, error) {
	return false, nil
}

func (*macaroonSuite) TestNoCaveats(c *gc.C) {
	rootKey := []byte("secret")
	m := macaroon.New(rootKey, "some id", "a location")
	c.Assert(m.Location(), gc.Equals, "a location")
	c.Assert(string(m.Id()), gc.Equals, "some id")

	ok, err := m.Verify(nil, rootKey, never, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)
}

func (*macaroonSuite) TestFirstPersonCaveat(c *gc.C) {
	rootKey := []byte("secret")
	m := macaroon.New(rootKey, "some id", "a location")

	caveats := map[string]bool{
		"a caveat": true,
		"another caveat": true,
	}
	tested := make(map[string]bool)

	for cav := range caveats {
		m.AddFirstPersonCaveat(cav)
	}

	check := func(cav string) (bool, error) {
		tested[cav] = true
		return caveats[cav], nil
	}
	ok, err := m.Verify(nil, rootKey, check, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)

	c.Assert(tested, gc.DeepEquals, caveats)
}