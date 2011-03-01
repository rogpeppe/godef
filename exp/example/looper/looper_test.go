package looper

import (
	"testing"
)

func TestLooper(t *testing.T) {
	c := make(chan bool)
	loop := NewLooper(func() bool {
		c <- true
		return false
	})
	<-c
	loop.Close()
}

func BenchmarkLooper(b *testing.B) {
	i := b.N - 1
	c := make(chan int)
	loop := NewLooper(func() bool {
		i--
		if i <= 0 {
			close(c)
			return false
		}
		return true
	})
	for _ = range c {
	}
	loop.Close()
}
