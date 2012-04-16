// Looper is an example package demonstrating use of the callback
// package. When a new Looper type is made, a new pthread is
// created which continually loops calling the callback function
// until it returns false.
package looper

//#include <pthread.h>
//#include <unistd.h>
//#define nil ((void*)0)
//typedef struct context context;
//struct context {
//	void *looper;
//	int ret;
//};
//
//extern void loopCallback(void*);
//
//// callback holds the callback library function.
//// It is stored in a function pointer because C linkage
//// does not work across packages.
//static void(*callback)(void (*f)(void*), void*);
//
//void loopInit(void *ptr){
//	callback = ptr;
//}
//
//// loop is the pthread function that actually runs the loop.
//// Note the context struct which is used both to pass
//// the argument and get the result.
//void *loop(void*looper){
//	context ctxt;
//	ctxt.looper = looper;
//	ctxt.ret = 0;
//	do{
//		callback(loopCallback, &ctxt);
//	}while(ctxt.ret);
//	return nil;
//}
//
//// startLooper creates the pthread.
//void startLooper(void *arg){
//	pthread_t tid;
//	pthread_create(&tid, nil, loop, arg);
//}
//
import "C"
import (
	"code.google.com/p/rog-go/exp/callback"
	"sync"
	"unsafe"
)

// The loopers map stores current looper instances.
// We use to make sure that the closure inside the
// Looper is not garbage collected - the reference inside
// the C code does not count.
var mutex sync.Mutex
var loopers = make(map[*Looper]bool)

func init() {
	// Get the callback function from the callback
	// package and pass it to the local C code.
	C.loopInit(callback.Func)
}

type Looper struct {
	f func() bool
}

// NewLooper creates a new Looper that continually
// calls f until it returns false.
func NewLooper(f func() bool) *Looper {
	// save the new Looper to prevent it from being
	// inappropriately garbage collected.
	mutex.Lock()
	l := &Looper{f}
	loopers[l] = true
	mutex.Unlock()

	// start the looping pthread.
	C.startLooper(unsafe.Pointer(l))
	return l
}

// Close marks the Looper as not in use.
// It must be called after the looper function
// has returned false.
func (l *Looper) Close() {
	mutex.Lock()
	delete(loopers, l)
	mutex.Unlock()
}

//export loopCallback
func loopCallback(arg unsafe.Pointer) {
	ctxt := (*C.context)(arg)
	looper := (*Looper)(ctxt.looper)

	// call back the Go function, then set the
	// return result.
	if looper.f() {
		ctxt.ret = 1
	} else {
		ctxt.ret = 0
	}
}
