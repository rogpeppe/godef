/*
 * An example C API, pretending to be some kind of window system.
 * A window can be created, which creates an associated event loop
 * running in another thread; a callback can be registered that will
 * be called when an event fires.
 */

#include <stdlib.h>
#include <pthread.h>
#include "window.h"

#define nil ((void*)0)

static int
fireEvent(Window *w, int event) {
	int shutdown;
	pthread_mutex_lock(&w->mu);
	shutdown = w->shutdown;
	if(!shutdown && w->handler != nil){
		w->handler(w->handlerCtxt, event);
	}
	pthread_mutex_unlock(&w->mu);
	return !shutdown;
}

static void *
eventLoop(void *arg) {
	Window *w;
	w = arg;
	for(;;){
		sleep(1);
		if(!fireEvent(w, someEvent)) {
			break;
		}
		sleep(1);
		if(!fireEvent(w, otherEvent)) {
			break;
		}
	}
	return nil;
}

Window*
newWindow() {
	Window *w;
	pthread_t tid;
	w = malloc(sizeof(Window));
	w->handler = nil;
	w->handlerCtxt = nil;
	w->shutdown = 0;
	pthread_mutex_init(&w->mu, nil);
	pthread_create(&tid, nil, eventLoop, w);
	return w;
}

void
windowClose(Window *w) {
	pthread_mutex_lock(&w->mu);
	w->shutdown = 1;
	pthread_mutex_unlock(&w->mu);
}

void
windowSetCallback(Window *w, void (*handler)(void *, int), void *ctxt) {
	pthread_mutex_lock(&w->mu);
	w->handler = handler;
	w->handlerCtxt = ctxt;
	pthread_mutex_unlock(&w->mu);
}
