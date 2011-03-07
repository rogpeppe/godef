typedef struct Window Window;
struct Window {
	pthread_mutex_t mu;
	void (*handler)(void *ctxt, int eventType);
	void *handlerCtxt;
	int shutdown;
};
Window *newWindow();
void windowClose(Window *w);
void windowSetCallback(Window *w, void (*handler)(void *, int), void *ctxt);
enum {
	someEvent,
	otherEvent,
};
