package abc

import (
	"fmt"
	"sync"
)

var StatusT = &Type{
	"status",
	true,
	IsType((*StatusManager)(nil)),
}

type StatusManager struct {
	lock    sync.Mutex
	running int
	waiting int
	wakeup  chan bool
}

type Status struct {
	m *StatusManager
}

func (m *StatusManager) Go(fn func(status *Status)) {
	m.lock.Lock()
	m.running++
	m.lock.Unlock()
	go func() {
		defer func() {
			switch x := recover().(type) {
			case string:
				fmt.Printf("caught panic: %s\n", x)
			case nil:
			default:
				panic(x)
			}
			// TODO: catch panic and turn it into a status message
			m.lock.Lock()
			m.running--
			if m.running == 0 && m.waiting > 0 {
				for ; m.waiting > 0; m.waiting-- {
					m.wakeup <- true
				}
				m.wakeup = make(chan bool)
			}
			m.lock.Unlock()
		}()
		fn(new(Status))
	}()
}

func (status *Status) Log(s string) {
}

func (status *Status) Error(s string) {
}

func (status *Status) Fail(e error) {
}

func (status *Status) Go(fn func(status *Status)) {
	status.m.Go(fn)
}

func (m *StatusManager) Wait() error {
	wait := false
	m.lock.Lock()
	if m.wakeup == nil {
		m.wakeup = make(chan bool)
	}
	c := m.wakeup
	if m.running > 0 {
		m.waiting++
		wait = true
	}
	m.lock.Unlock()
	if wait {
		<-c
	}
	return nil
}
