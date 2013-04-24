package main_test
import (
	. "launchpad.net/gocheck"
	"testing"
)

func TestPackage(t *testing.T) {
	TestingT(t)
}

type suite struct{}

var _ = Suite(suite{})

func (suite) 

func initRepo(c *C, dir, kind string) {
	// This relies on the fact that hg, bzr and 
	err := runCmd(dir, kind, "init")
	c.Assert(err, IsNil)
}
