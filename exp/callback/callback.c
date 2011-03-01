#include <pthread.h>
#include <stdlib.h>
#include "callback.h"
#define nil ((void*)0)

typedef struct Callback Callback;
struct Callback {
	void (*f)(void*);
	void *arg;
	int done;
	pthread_cond_t cond;
	Callback *next;
};

extern void newCallbackRunner(void);

static pthread_cond_t cbcond;		// Condition to signal that there is a callback to run.
static pthread_mutex_t cbmutex;	// Guards the following global variables.
static Callback *callbacks;			// List of outstanding callbacks.
static int idlecount;				// Number of available waiting threads.
static Callback *freelist;			// Recycled Callback structures.

void
callbackInit(void){
	// These variables need to be explicitly initialised to guard
	// against issue 1559.
	callbacks = nil;
	idlecount = 1;		// one waiter is started automatically by init.
	freelist = nil;
	pthread_mutex_init(&cbmutex, nil);
	pthread_cond_init(&cbcond, nil);
}

// runCallbacks sits forever waiting for new callbacks,
// and then running them. It makes sure that there
// is always a ready instance of runCallbacks by
// starting a new one when the idle count goes to zero.
void
runCallbacks(void){
	Callback *item;
	pthread_mutex_lock(&cbmutex);
	for(;;){
		// Wait for a callback to arrive.
 		while(callbacks == nil){
			pthread_cond_wait(&cbcond, &cbmutex);
		}
		item = callbacks;
		callbacks = callbacks->next;
		item->next = nil;
		// Decrement the idle count while we're running
		// the function. If it goes to zero, then we start
		// a new thread.
		if(--idlecount == 0){
			idlecount++;
			pthread_mutex_unlock(&cbmutex);
			newCallbackRunner();
		}else{
			pthread_mutex_unlock(&cbmutex);
		}

		// Wake the next waiter.
		pthread_cond_signal(&cbcond);

		// Call back the function.
		item->f(item->arg);

		pthread_mutex_lock(&cbmutex);

		// Wake up the caller.
		item->done = 1;
		pthread_cond_signal(&item->cond);
		idlecount++;
	}
}

// Call f with the given argument in a Go context, even
// if the current thread has not been created by Go.
void
callback(void (*f)(void*), void*arg){
	Callback *item;
	pthread_mutex_lock(&cbmutex);
	if(freelist != nil){
		item = freelist;
		freelist = freelist->next;
	}else{
		item = malloc(sizeof(Callback));
		pthread_cond_init(&item->cond, nil);
	}
	item->f = f;
	item->arg = arg;
	item->done = 0;
	item->next = nil;

	item->next = callbacks;
	callbacks = item;
	pthread_cond_signal(&cbcond);
	while(!item->done){
		pthread_cond_wait(&item->cond, &cbmutex);
	}
	item->next = freelist;
	freelist = item;
	pthread_mutex_unlock(&cbmutex);
}

// this exists because we cannot get function pointers directly
// from Go.
void*
callbackFunc(void) {
	return callback;
}
