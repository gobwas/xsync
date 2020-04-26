package xsync

import (
	"sync"
)

// A Mutex is a mutual exclusion lock with ability to be locked with priority
// and/or cancellation.
// The zero value for a Mutex is an unlocked mutex.
//
// A Mutex must not be copied after first use.
type Mutex struct {
	mu     sync.Mutex
	once   sync.Once
	cond   Cond
	locked bool
}

func (m *Mutex) init() {
	m.once.Do(func() {
		m.cond = Cond{
			L: &m.mu,
		}
	})
}

// Lock locks m.
//
// Unlike sync.Mutex if lock is already in use Lock() can return before
// Unlock() if and only if given demand's cancellation channel became filled of
// closed. In that case returned err is ErrCanceled.
func (m *Mutex) Lock(d Demand) error {
	m.init()

	m.mu.Lock()
	defer m.mu.Unlock()
	for m.locked {
		if err := m.cond.Wait(d); err != nil {
			return ErrCanceled
		}
	}
	m.locked = true

	return nil
}

// Unlock unlocks m.
// It panics if m is not locked on entry to Unlock().
func (m *Mutex) Unlock() {
	m.mu.Lock()
	if !m.locked {
		m.mu.Unlock()
		panic("xsync: Unlock() of unlocked mutex")
	}
	m.locked = false
	m.mu.Unlock()
	m.cond.Signal()
}
