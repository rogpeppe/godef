// The event package demonstrates use of the callback package
// to call Go functions from a non-Go-created thread.
// The C "window" API is intended to represent a conventional
// C API which invokes callbacks from a thread it has created itself.
// The event package layers a Go callback on top of that.
package event

//#include <pthread.h>
//#include <unistd.h>
//#include "window.h"
//#define nil ((void*)0)
//typedef struct args args;
//struct args {
//	void *goWindow;
//	int event;
//};
//extern void eventCallback(void*);
//
//// goCallback holds the function from the callback package.
//// It is stored in a function pointer because C linkage
//// does not work across packages.
//static void(*goCallback)(void (*f)(void*), void*);
//
//void
//winInit(void *callbackFunc){
//	goCallback = callbackFunc;
//}
//static void
//localCallback(void *goWindow, int event){
//	args a;
//	a.goWindow = goWindow;
//	a.event = event;
//	(*goCallback)(eventCallback, &a);
//}
//
//void
//setLocalCallback(Window *w, void *goWindow){
//	windowSetCallback(w, localCallback, goWindow);
//}
//
import "C"
import (
	"code.google.com/p/rog-go/exp/callback"
	"unsafe"
)

func init() {
	// Get the callback function from the callback
	// package and pass it to the local C code.
	C.winInit(callback.Func)
}

type Window struct {
	w        *C.Window
	callback func(event int)
}

func NewWindow() *Window {
	w := C.newWindow()
	return &Window{w, nil}
}

func (w *Window) SetCallback(f func(int)) {
	// disable callbacks while we set up the go callback function.
	C.windowSetCallback(w.w, nil, nil)
	w.callback = f
	C.setLocalCallback(w.w, unsafe.Pointer(w))
}

//export eventCallback
func eventCallback(a unsafe.Pointer) {
	arg := (*C.args)(a)
	w := (*Window)(arg.goWindow)
	w.callback(int(arg.event))
}
