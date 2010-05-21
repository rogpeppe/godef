package main
import "sync"

type Barrier struct {
	lock sync.Mutex
	n int					// number  of current processes waiting 
	total int				// number of processes in barrier
	done chan bool
}

// NewBarrier creates a barrier object that admits total processes.
func NewBarrier(total int) *Barrier {
	return &Barrier{total: total, done: make(chan bool)}
}

// Wait blocks until all processes have called Wait().
func (b *Barrier) Wait() {
	b.lock.Lock()
	b.n++
	if b.n < b.total {
		b.lock.Unlock()
		// wait for the other processes to arrive.
		<-b.done
	}else{
		// we're the last process; tell all the other
		// waiting processes that they can continue.
		for i := 0; i < b.n - 1; i++ {
			b.done <- true
		}
		b.n = 0
		b.lock.Unlock();
	}
}

func (b *Barrier) Enter() {
	b.lock.Lock()
	b.total++
	b.lock.Unlock()
}

func (b *Barrier) Leave() {
	b.lock.Lock()
	b.total--
	b.lock.Unlock()
}
