package xsync

import (
	"sync"
	"sync/atomic"

	"github.com/gobwas/xsync/internal/buildtags"
)

// list is a ticket-based prioritized notification list.
//
// Ticket-based means that a caller first calls Add() to receive a list ticket
// and then eventually calls Wait() with it. Receiving ticket gives ability to
// fix caller's position in the queue under its own synchronization and then
// call Wait() without.
//
// list maintains atomic counter which consist of two parts – first one is
// number of goroutines in transition from Add() to Wait() (pending count);
// second is a monotonically increasing ticked id.
// We need to provide that pending counter to be able to answer such question
// during Notify() processing: if the list's heap is empty now – does it mean
// that list is empty, or we just won the race with some goroutine in
// transition?
//
// For simplicity of logic and implementation list's Notify() does not wait for
// pending goroutines if there is at least one element in the heap. Such
// behavior can lead to not complete fairness – suppose that G1 is in the heap
// right now, and G2 is in the transition; G1 has much lower priority; G1 has
// ticket older than G2's; but G2 loose the race with G1's Wait() and some
// other's Notify(). From the one hand – we must wait for G2 and signal it
// instead of G1, but from the other hand – we do not know exactly about G2's
// priority until we receive it in the Wait(). That is, if G2 is lower than G1,
// we could delay G1's signaling without any reason.
type list struct {
	mu sync.Mutex

	heap wgHeap
	wait dcounter // Atomic counter of [ pending uint32, waiters uint32 ].

	// done is the number of signaled/canceled tickets.
	// It may be changed atomically only under the mu lock.
	// It is possible to atomically read it without taking the lock.
	done  uint32
	inbox uint32

	// NOTE: for complete fairness list may hold cond's sync.Locker and Lock()
	// it before sending event into wg.ch.

	// These hooks are called only if debug buildtag passed.
	hookInsert func(*wg)
	hookNotify func(*wg)
}

// Add returns list ticket without locking.
func (l *list) Add() (ticket uint32) {
	// Increment both counters and use waiters value as a new ticket.
	_, t := l.wait.Add(
		1, // Number of tickets in transition (pending) from Add() to Wait().
		1, // Ticket counter.
	)
	return t
}

// Wait blocks caller until notification received.
func (l *list) Wait(ticket uint32, d Demand) bool {
	g := acquireG()
	g.t = ticket
	g.p = d.Priority

	l.mu.Lock()

	// Caller arrived with ticket. Decrement the pending part of a counter by
	// one.
	//
	// NOTE: we decrementing counter only under the mutex.
	// That is, received pending count may only grow until we release the
	// mutex.
	l.wait.Add(minusOne, 0)
	if l.inbox > 0 {
		l.inbox--
		if buildtags.Debug {
			if l.hookNotify != nil {
				l.hookNotify(g)
			}
		}
		l.mu.Unlock()
		return true
	}

	// Insert ticket into the list and block until notification.
	l.heap.Push(g)
	if buildtags.Debug {
		if hook := l.hookInsert; hook != nil {
			hook(g)
		}
	}
	l.mu.Unlock()

	var cancel <-chan struct{}
	if d.Cancel != nil {
		cancel = d.Cancel
	}
	if cancel == nil {
		<-g.c
		releaseG(g)
		return true
	}
	select {
	case <-g.c:
		releaseG(g)
		return true

	case <-cancel:
		l.mu.Lock()
		evicted := l.heap.Remove(g)
		if evicted {
			atomic.AddUint32(&l.done, 1)
		}
		l.mu.Unlock()

		if !evicted {
			// Ticket was already removed from the list. That is, it means that
			// it was notified successfully and we loose the race on removing g
			// from list with notification process. We must interpret this case
			// like successful await, and not drop the notification.
			return true
		}

		// G is successfully evicted from the list thus we can reuse it to
		// reduce the pressure on GC and allocator.
		releaseG(g)

		return false
	}
}

func (l *list) Notify() {
	_, last := l.wait.Load()
	if equalbits(last, atomic.LoadUint32(&l.done)) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Re-check under the lock if we need to do something.
	// Invariants:
	//   - pending can grow;
	//   - done can not change.
	pending, last := l.wait.Load()
	done := atomic.LoadUint32(&l.done)
	if equalbits(last, done) {
		return
	}

	g := l.heap.Pop()
	if g == nil && pending > l.inbox {
		// No landed tickets yet.
		// Increase the inbox counter to let them grab it immediately.
		atomic.AddUint32(&l.done, 1)
		l.inbox++
		return
	}
	if g == nil && pending == 0 {
		panic("xsync: inconsistent list state")
	}
	if buildtags.Debug {
		if l.hookNotify != nil {
			l.hookNotify(g)
		}
	}
	g.notify()
	atomic.AddUint32(&l.done, 1)
}

// TODO: test notify all while there are pending waiters – land pendings after
// NotifyAll() and call NotifyOne().
//
func (l *list) NotifyAll() {
	_, last := l.wait.Load()
	if equalbits(last, atomic.LoadUint32(&l.done)) {
		return
	}

	l.mu.Lock()

	// Re-check under the lock if we need to do something.
	// Invariants:
	//   - pending can grow;
	//   - done can not change.
	pending, last := l.wait.Load()
	done := atomic.LoadUint32(&l.done)
	if equalbits(last, done) {
		l.mu.Unlock()
		return
	}

	prev := l.inbox
	l.inbox = pending
	h := l.heap
	l.heap = wgHeap{}

	atomic.AddUint32(&l.done, uint32(h.Size())+(l.inbox-prev))

	l.mu.Unlock()

	for _, g := range h.data {
		if buildtags.Debug {
			if l.hookNotify != nil {
				l.hookNotify(g)
			}
		}
		g.notify()
	}
}

var gp sync.Pool

type wg struct {
	c chan struct{}
	t uint32
	p Priority

	// pos is the position within the list's heap.
	// It MUST be populated with h.mu held.
	pos int
}

func (g *wg) less(b *wg) bool {
	return compare(g, b) < 0
}

func compare(g1, g2 *wg) (c int) {
	// TODO: fix low priority tickets starvation case.
	// Low priority tickets could starve even for current counter generation
	// change. We must detect the change of the generation of the current
	// counter and release the lower priority tickets having previous
	// generation.
	if c = comparePriority(g1.p, g2.p); c == 0 {
		// Demands are equal so compare tickets but in reverse order – least
		// ticket is the topmost item in the queue.
		c = comparebits(g2.t, g1.t)
	}
	return
}

func invert(c int) int {
	return ^c + 1
}

func (g *wg) notify() {
	g.c <- struct{}{}
}

func acquireG() *wg {
	if g, ok := gp.Get().(*wg); ok {
		return g
	}
	return &wg{
		c:   make(chan struct{}, 1),
		pos: -1,
	}
}

func releaseG(g *wg) {
	c := g.c
	*g = wg{
		c:   c,
		pos: -1,
	}
	gp.Put(g)
}
