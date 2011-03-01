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
//static void(*callback)(void (*f)(void*), void*);
//void tstInit(void *ptr){
//	callback = ptr;
//}
//
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
//	
//void startLooper(void *arg){
//	pthread_t tid;
//	pthread_create(&tid, nil, loop, arg);
//}
//
import "C"
import (
	"rog-go.googlecode.com/hg/exp/callback"
	"sync"
	"unsafe"
)

var mutex sync.Mutex
var loopers = make(map[*Looper]bool)

func init(){
	C.tstInit(callback.Func);
}

type Looper struct {
	f func() bool
}

func NewLooper(f func() bool) *Looper {
	mutex.Lock()
	l := &Looper{f}
	loopers[l] = true
	mutex.Unlock()
	C.startLooper(unsafe.Pointer(l))
	return l
}

func (l *Looper) Close() {
	mutex.Lock()
	loopers[l] = false, false
	mutex.Unlock()
}

//export loopCallback
func loopCallback(arg unsafe.Pointer) {
	ctxt := (*C.context)(arg);
	looper := (*Looper)(ctxt.looper);
	if looper.f() {
		ctxt.ret = 1;
	}else{
		ctxt.ret = 0;
	}
}
