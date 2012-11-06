package event

import (
	"fmt"
	"testing"
	"time"
)

func TestEvents(t *testing.T) {
	w := NewWindow()
	w.SetCallback(func(event int) {
		fmt.Printf("got event %d\n", event)
	})
	time.Sleep(5e9)
}
