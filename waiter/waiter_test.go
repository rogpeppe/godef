package waiter

import (
	"testing"
	"time"
)

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
		for i := 0; i < N; i++ {
			start <- true
		}
		w.Wait()
		for i := 0; i < N+1; i++ {
			<-end
		}
	}
}
