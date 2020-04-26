package xsync

import (
	"testing"
)

func TestMutex(t *testing.T) {
	skipWithoutDebug(t)

	var m Mutex
	m.init()

	insert := make(chan struct{})
	m.cond.list.hookInsert = func(*wg) {
		insert <- struct{}{}
	}

	// Lock the mutex.
	if err := m.Lock(Demand{}); err != nil {
		t.Fatal(err)
	}

	type lockAndErr struct {
		i   int
		err error
	}
	locked := make(chan lockAndErr, 10)
	for i := 0; i < cap(locked); i++ {
		go func(i int) {
			err := m.Lock(Demand{})
			locked <- lockAndErr{
				i:   i,
				err: err,
			}
			if err == nil {
				m.Unlock()
			}
		}(i)

		<-insert
		m.mu.Lock()
		m.mu.Unlock()
	}

	select {
	case <-locked:
		t.Fatalf("goroutine is not asleep")
	default:
	}

	m.Unlock()

	for i := 0; i < cap(locked); i++ {
		x := <-locked
		if x.err != nil {
			t.Fatalf("Lock() error: %v", x.err)
		}
		if act, exp := x.i, i; act != exp {
			t.Errorf("wrong goroutine woke up: %d; want %d", act, exp)
		}
	}
}
