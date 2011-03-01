package callback

//#include "callback.h"
import "C"
import (
	"runtime"
	"unsafe"
)

//export newCallbackRunner
func newCallbackRunner() {
	go C.runCallbacks()
}

func init() {
	// work around issue 1560.
	if runtime.GOMAXPROCS(0) < 2 {
		runtime.GOMAXPROCS(2)
	}

	C.callbackInit()
	go C.runCallbacks()
}

// Func holds a pointer to the C callback function.
// It can be used by converting it to a function pointer
// with type void (*callback)(void (*f)(void*), void *arg);
// When called, it calls the provided function f in a
// a Go context.
var Func = unsafe.Pointer(C.callbackFunc())
