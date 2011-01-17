package waiter

import "sync"

// A Waiter waits for a collection of goroutines to finish.
// Add is called to add goroutines to the collection.
// Then each of the goroutines runs and calls Done when finished.
// At the same time, Wait can be used to block until all the goroutines finish.
//
// e.g. to start a collection of goroutines and wait for them to complete:
//	var w Waiter
//	w.Add(10)
//	for i := 0; i < 10; i++ {
//		go func() {
//       		doSomething()
//			w.Done()
//		}()
//	}
//	w.Wait()

type Waiter struct {
	lock sync.Mutex
	done chan bool
	n    int
}

// Add adds n goroutines to the Waiter. It may
// be called concurrently with Wait.
func (w *Waiter) Add(n int) {
	if n < 0 {
		panic("waiter: negative add count")
	}
	w.lock.Lock()
	w.n += n
	w.lock.Unlock()
}

// Done removes a goroutine from the Waiter.
func (w *Waiter) Done() {
	w.lock.Lock()
	w.n--
	if w.n < 0 {
		panic("waiter: mismatched add/done")
	}
	if w.n == 0 && w.done != nil {
		w.done <- true
		w.done = nil
	}
	w.lock.Unlock()
}

// Wait waits for the collection of goroutines to empty.
// It may be called multiple times concurrently.
func (w *Waiter) Wait() {
	w.lock.Lock()
	if w.n == 0 {
		w.lock.Unlock()
		return
	}
	if w.done == nil {
		w.done = make(chan bool, 1)
	}
	c := w.done
	w.lock.Unlock()
	<-c
	c <- true
}
