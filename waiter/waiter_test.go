package waiter

import (
	"testing"
	"time"
)

func testWaiter(t *testing.T, b1 *Waiter, b2 *Waiter) {
	n := 16
	b1.Add(n)
	b2.Add(1)
	exited := make(chan bool)
	max := 0
	for i := 0; i != n; i++ {
		go func(i int) {
			max = i
			b1.Done()
			b2.Wait()
			exited <- true
		}(i)
	}
	b1.Wait()
	if _, ok := <-exited; max != n-1 || ok {
		t.Fatal("Waiter didn't work")
	}
	b2.Done()
	for i := 0; i != n; i++ {
		<-exited // Will block if barrier fails to unlock someone.
	}
}

func TestWaiter(t *testing.T) {
	b1 := &Waiter{}
	b2 := &Waiter{}

	// Run the same test twice to ensure barrier is left in a proper state.
	testWaiter(t, b1, b2)
	testWaiter(t, b1, b2)
}

func TestWaiterMisuse(t *testing.T) {
	defer func() {
		err := recover()
		if err != "waiter: mismatched add/done" {
			t.Fatalf("Unexpected panic: %#v", err)
		}
	}()
	b := &Waiter{}
	b.Add(1)
	b.Done()
	b.Done()
	t.Fatal("Should panic")
}

func TestNone(t *testing.T) {
	var w Waiter
	w.Wait()
}

func TestOne(t *testing.T) {
	var w Waiter
	w.Add(1)
	w.Done()
	w.Wait()
}

func TestSeveral(t *testing.T) {
	const N = 10
	var w Waiter
	for r := 0; r < 4; r++ {
		start := make(chan bool)
		end := make(chan bool)
		w.Add(1)
		go func() {
			w.Wait()
			end <- true
		}()
		time.Sleep(0.05e9)			// make sure first Wait has started
		for i := 0; i < N; i++ {
			w.Add(1)
			go func() {
				<-start
				w.Done()
				end <- true
			}()
		}
		w.Done()
		for i := 0; i < N; i++ {
			start <- true
		}
		w.Wait()
		for i := 0; i < N+1; i++ {
			<-end
		}
	}
}
