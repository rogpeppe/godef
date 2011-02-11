include $(GOROOT)/src/Make.inc

TARG=rog-go.googlecode.com/hg/exp/abc
GOFILES=\
	abc.go\
	status.go\
	transform.go\
	y.go\

include $(GOROOT)/src/Make.pkg

y.go: abc.y
	goyacc abc.y
