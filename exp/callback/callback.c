#include <pthread.h>
#include <stdarg.h>
#include <stdio.h>
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

static pthread_cond_t cbcond;		// Condition to signal a callback to run.
static pthread_mutex_t cbmutex;	// Guards the following global variables.
static Callback *callbacks;			// List of outstanding callbacks.
static int cbwaiters;				// Number of available waiting threads.
static Callback *freelist;			// Recycled Callback structures.

void
callbackInit(void){
	// These variables need to be explicitly initialised to guard
	// against a bug in cgo under mac os.
	callbacks = nil;
	cbwaiters = 1;		// one waiter is started automatically by init.
	freelist = nil;
	pthread_mutex_init(&cbmutex, nil);
	pthread_cond_init(&cbcond, nil);
}

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
		if(--cbwaiters == 0){
			cbwaiters++;
			pthread_mutex_unlock(&cbmutex);
			newCallbackRunner();
		}else{
			pthread_mutex_unlock(&cbmutex);
		}

		pthread_cond_signal(&cbcond);	// wake the next waiter
		item->f(item->arg);
		pthread_mutex_lock(&cbmutex);
		item->done = 1;
		pthread_cond_signal(&item->cond);
		cbwaiters++;
	}
}

// Call f with the given argument in a Go context,
// even if the current thread has been created from without
// Go.
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

static void
print(char *fmt, ...){
	char buf[50];
	int n;
	va_list ap;
	va_start(ap, fmt);
	n = snprintf(buf, sizeof(buf), "%p ", pthread_self());
	n += vsnprintf(buf+n, sizeof(buf)-n, fmt, ap);
	va_end(ap);
	if(n < sizeof(buf)){
		buf[n++] = '\n';
	}
	write(1, buf, n);
}
