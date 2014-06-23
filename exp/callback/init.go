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
// When called, it calls the provided function f in a
// a Go context with the given argument.
//
// It can be used by first converting it to a function pointer
// and then calling from C.
// Here is an example that sets up the callback function:
// 	//static void (*callback)(void (*f)(void*), void *arg);
//	//void setCallback(void *c){
//	//	callback = c;
//	//}
//     import "C"
//	import "code.google.com/p/rog-go/exp/callback"
//	
//	func init() {
//		C.setCallback(callback.Func)
//	}
//
var Func = callbackFunc

var callbackFunc = unsafe.Pointer(C.callbackFunc())
