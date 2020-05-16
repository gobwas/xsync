/*
Package xsync provides priortizable and cancellable synchronization primitives
such as condition variable.
*/
package xsync

import (
	"errors"
	"sync"
)

// Errors returned by package structs.
var (
	ErrCanceled = errors.New("xsync: canceled")
	ErrClosed   = errors.New("xsync: closed")
)

// Cond contains logic of checking condition and waiting for a condition
// change.
//
// Waiting for condition change could be done with cancelation channel.
// This is the main intention of this cond existence.
type Cond struct {
	L    sync.Locker
	list list
}

// Wait unlocks c.L and suspends execution of the calling goroutine.
//
// Unlike sync.Cond Wait() can return before awoken by Signal() if
// and only if given cancelation channel become filled or closed.
// In that case returned err is ErrCanceled.
//
// After later resume of execution, Wait() locks c.L before returning.
func (c *Cond) Wait(d Demand) (err error) {
	// Receive wait list ticker under locked L to serialize the waiter queue.
	ticket := c.list.Add()
	c.L.Unlock()
	if !c.list.Wait(ticket, d) {
		err = ErrCanceled
	}
	c.L.Lock()
	return err
}

// Signal wakes n goroutines waiting on c, if there is any.
func (c *Cond) Signal() {
	c.list.Notify()
}

// Broadcast wakes all goroutines waiting on c.
func (c *Cond) Broadcast() {
	c.list.NotifyAll()
}
