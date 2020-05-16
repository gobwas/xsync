package xsync

import (
	"sync"
	"sync/atomic"
)

type task struct {
	t        Task
	priority Priority
	seq      uint64
	pos      int // intrusive heap index.
}

func (t task) less(b task) bool {
	c := comparePriority(t.priority, b.priority)
	if c != 0 {
		return c < 0
	}
	return t.seq < b.seq
}

type queue struct {
	mu     sync.Mutex
	snd    Cond
	rcv    Cond
	heap   taskHeap
	ticket uint64
	closed bool
}

func (q *queue) init(n int) {
	if n <= 0 {
		panic("queue size must be >0")
	}
	q.heap.Reserve(n)
	q.snd.L = &q.mu
	q.rcv.L = &q.mu
}

func (q *queue) send(d Demand, t Task) error {
	seq := atomic.AddUint64(&q.ticket, 1)

	q.mu.Lock()
	defer q.mu.Unlock()

	for !q.closed && q.heap.IsFull() {
		err := q.snd.Wait(d)
		if err != nil {
			return err
		}
	}
	if q.closed {
		return ErrClosed
	}
	q.heap.Push(task{
		t:        t,
		priority: d.Priority,
		seq:      seq,
	})
	q.rcv.Signal()

	return nil
}

func (q *queue) recv(d Demand) (Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for !q.closed && q.heap.IsEmpty() {
		err := q.rcv.Wait(d)
		if err != nil {
			return nil, err
		}
	}
	if q.heap.Size() > 0 {
		t := q.heap.Pop()
		q.snd.Signal()
		return t.t, nil
	}
	if q.closed {
		return nil, ErrClosed
	}

	panic("goros: inconsistent queue")
}

// recvTo receives up to len(xs) elements from queue.
func (q *queue) recvTo(xs []Task, cancel <-chan struct{}) (n int, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for !q.closed && q.heap.IsEmpty() {
		err := q.rcv.Wait(Demand{
			Cancel: cancel,
		})
		if err != nil {
			return 0, err
		}
	}

	n = min(q.heap.Size(), len(xs))
	for i := 0; i < n; i++ {
		t := q.heap.Pop()
		xs[i] = t.t
		q.snd.Signal()
	}
	if n == 0 && q.closed {
		return 0, ErrClosed
	}

	return n, nil
}

func (q *queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		q.snd.Broadcast()
		q.rcv.Broadcast()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
