include $(GOROOT)/src/Make.$(GOARCH)

TARG=bounce
GOFILES=\
	canvas.go\
	barrier.go\
	bounce.go\

include $(GOROOT)/src/Make.cmd
