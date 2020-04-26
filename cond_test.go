package xsync

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestCondWaitCancelation(t *testing.T) {
	var mu sync.Mutex
	c := &Cond{L: &mu}

	var (
		cancel  = make(chan struct{})
		result  = make(chan error, 2)
		waiting = make(chan struct{}, 2)
	)
	for i := 0; i < 2; i++ {
		go func() {
			mu.Lock()
			waiting <- struct{}{}
			result <- c.Wait(Demand{
				Cancel: cancel,
			})
			mu.Unlock()
		}()
	}

	// Ensure that all waiters are waiting.
	for i := 0; i < 2; i++ {
		<-waiting
	}
	// Lock/Unlock the mutex to ensure that all waiters are blocked on Wait()
	// call.
	mu.Lock()
	mu.Unlock()

	// Wake up first waiter with Signal().
	c.Signal()
	// Wake up the second waiter with cancelation.
	close(cancel)

	var err error
	for i := 0; i < 2; i++ {
		select {
		case e := <-result:
			if e != nil && e != ErrCanceled {
				t.Fatalf("unexpected error: %v", e)
			}
			if e != nil && err != nil {
				t.Fatalf("unexpected second error")
			}
			if e != nil && err == nil {
				err = e
			}
		case <-time.After(time.Second):
			t.Errorf("no result after 1s")
		}
	}
}

func TestCondSignal(t *testing.T) {
	var mu sync.Mutex
	c := &Cond{L: &mu}

	const n = 200
	var (
		queue = make(chan int, n)
		awake = make(chan int, n)
	)
	for i := 0; i < n; i++ {
		go func(i int) {
			mu.Lock()
			queue <- i
			c.Wait(Demand{})
			awake <- i
			mu.Unlock()
		}(i)
		// Ensure that i-th waiter is on Wait() call.
		<-queue
		// Lock/Unlock the mutex to ensure that all waiters are blocked on
		// Wait() call.
		mu.Lock()
		mu.Unlock()
	}
	select {
	case <-awake:
		t.Fatalf("goroutine is not asleep")
	default:
	}
	for i := 0; i < n; i++ {
		c.Signal()
		select {
		case act := <-awake:
			if act != i {
				t.Errorf("wrong goroutine woke up: %d; want %d", act, i)
			}
		case <-time.After(time.Second):
			t.Fatalf("no awoken goroutine after 1s")
		}

		// Let the possible error happen.
		runtime.Gosched()

		select {
		case <-awake:
			t.Fatalf("too many goroutines awake")
		default:
		}
	}
}

func TestCondBroadcast(t *testing.T) {
	skipWithoutDebug(t)

	const n = 10
	var (
		queue  = make(chan int)
		awake  = make(chan int, n)
		repeat = make(chan struct{})
	)

	var mu sync.Mutex
	c := &Cond{
		L: &mu,
		list: list{
			hookInsert: func(g *wg) {
				queue <- int(g.t) - 1
			},
			hookNotify: func(g *wg) {
				awake <- int(g.t) - 1
			},
		},
	}

	for i := 0; i < n; i++ {
		go func(g int) {
			// NOTE: we doing Wait() twice here.
			for j := 0; j < 2; j++ {
				mu.Lock()
				c.Wait(Demand{})
				mu.Unlock()
				<-repeat
			}
		}(i)
		// Ensure that i-th waiter is on Wait().
		<-queue
	}
	// Lock/Unlock the mutex to ensure that all waiters are blocked on
	// Wait() call.
	mu.Lock()
	mu.Unlock()

	select {
	case <-awake:
		t.Fatalf("goroutine is not asleep")
	default:
	}

	c.Broadcast()

	seen := make(map[int]bool, n)
	for i := 0; i < n; i++ {
		select {
		case act := <-awake:
			if act != i {
				t.Errorf("wrong goroutine woke up: %d; want %d", act, i)
			}
			if seen[act] {
				t.Errorf("goroutine woke up more than once: %d", act)
			} else {
				seen[act] = true
			}
		case <-time.After(time.Second):
			t.Fatalf("no awoken goroutine after 1s")
		}
	}

	// Let the all waiters to be queued at second time.
	close(repeat)
	for i := 0; i < n; i++ {
		// Ensure that i-th waiter is on Wait().
		<-queue
	}
	// Lock/Unlock the mutex to ensure that all waiters are blocked on
	// Wait() call.
	mu.Lock()
	mu.Unlock()

	// Let the possible error happen.
	runtime.Gosched()

	select {
	case <-awake:
		t.Fatalf("too many goroutines awake")
	default:
	}
}

func TestCondNotifyFairness(t *testing.T) {
	test := func(t *testing.T, n, m int, c *condwrap) []int {
		var (
			line = make([]int, 0, n*m)
			done = make(chan struct{}, n)

			x = 0
			y = 0
		)
		for i := 0; i < n; i++ {
			go func() {
				for i := 0; i < m; i++ {
					c.L.Lock()
					if x == -1 {
						c.L.Unlock()
						break
					}
					x++
					signal := x == n
					if !signal {
						y++
						z := y
						c.Wait()
						line = append(line, z)
					} else {
						x = 0
					}
					c.L.Unlock()
					if signal {
						for i := 0; i < n-1; i++ {
							c.Signal()
							runtime.Gosched()
						}
					}
				}
				c.L.Lock()
				x = -1
				c.L.Unlock()
				c.Broadcast()
				done <- struct{}{}
			}()
		}
		for i := 0; i < n; i++ {
			<-done
		}
		return line
	}
	for _, p := range []struct {
		parallelism int
		n           int
	}{
		{
			parallelism: 2,
			n:           10,
		},
		{
			parallelism: 10,
			n:           1000,
		},
	} {
		suffix := fmt.Sprintf("%d@%d", p.n, p.parallelism)
		t.Run("this "+suffix, func(t *testing.T) {
			var mu sync.Mutex
			c := Cond{L: &mu}

			list := test(t, p.parallelism, p.n, &condwrap{
				L:         &mu,
				Wait:      func() { c.Wait(Demand{}) },
				Signal:    func() { c.Signal() },
				Broadcast: func() { c.Broadcast() },
			})

			t.Logf("fairness: %v", orderRatio(list))
		})
		t.Run("sync "+suffix, func(t *testing.T) {
			var mu sync.Mutex
			c := &sync.Cond{L: &mu}

			list := test(t, p.parallelism, p.n, &condwrap{
				L:         &mu,
				Wait:      func() { c.Wait() },
				Signal:    func() { c.Signal() },
				Broadcast: func() { c.Broadcast() },
			})

			t.Logf("fairness: %v", orderRatio(list))
		})
	}
}

func orderRatio(list []int) (ratio float64) {
	var good int
	for i := 1; i < len(list); i++ {
		prev := list[i-1]
		next := list[i]
		if prev+1 == next {
			good++
		}
	}
	return float64(good) / float64(len(list)-1)
}

func BenchmarkCondNotify(b *testing.B) {
	bench := func(b *testing.B, n int, c *condwrap) {
		done := make(chan struct{}, n)
		x := 0
		for i := 0; i < n; i++ {
			go func() {
				for i := 0; i < b.N; i++ {
					c.L.Lock()
					if x == -1 {
						c.L.Unlock()
						break
					}
					x++
					signal := x == n
					if !signal {
						c.Wait()
					} else {
						x = 0
					}
					c.L.Unlock()
					if signal {
						for i := 1; i < n; i++ {
							c.Signal()
						}
					}
				}
				c.L.Lock()
				x = -1
				c.L.Unlock()
				c.Broadcast()
				done <- struct{}{}
			}()
		}
		for i := 0; i < n; i++ {
			<-done
		}
	}
	for _, n := range []int{2, 4, 8, 16, 32, 64, 128} {
		b.Run("this "+strconv.Itoa(n), func(b *testing.B) {
			var mu sync.Mutex
			c := Cond{L: &mu}
			bench(b, n, &condwrap{
				L:         &mu,
				Wait:      func() { c.Wait(Demand{}) },
				Signal:    func() { c.Signal() },
				Broadcast: func() { c.Broadcast() },
			})
		})
		b.Run("sync "+strconv.Itoa(n), func(b *testing.B) {
			var mu sync.Mutex
			c := &sync.Cond{L: &mu}
			bench(b, n, &condwrap{
				L:         &mu,
				Wait:      func() { c.Wait() },
				Signal:    func() { c.Signal() },
				Broadcast: func() { c.Broadcast() },
			})
		})
	}
}

func BenchmarkCondBroadcast(b *testing.B) {
	bench := func(b *testing.B, n int, c *condwrap) {
		done := make(chan struct{}, n)
		x := 0
		for i := 0; i < n; i++ {
			go func() {
				for i := 0; i < b.N; i++ {
					c.L.Lock()
					if x == -1 {
						c.L.Unlock()
						break
					}
					x++
					if x == n {
						x = 0
						c.Broadcast()
					} else {
						c.Wait()
					}
					c.L.Unlock()
				}
				c.L.Lock()
				x = -1
				c.L.Unlock()
				c.Broadcast()
				done <- struct{}{}
			}()
		}
		for i := 0; i < n; i++ {
			<-done
		}
	}
	for _, n := range []int{2, 4, 8, 16, 32, 64, 128} {
		b.Run("this "+strconv.Itoa(n), func(b *testing.B) {
			var mu sync.Mutex
			c := Cond{L: &mu}
			bench(b, n, &condwrap{
				L:         &mu,
				Wait:      func() { c.Wait(Demand{}) },
				Broadcast: func() { c.Broadcast() },
			})
		})
		b.Run("sync "+strconv.Itoa(n), func(b *testing.B) {
			var mu sync.Mutex
			c := &sync.Cond{L: &mu}
			bench(b, n, &condwrap{
				L:         &mu,
				Wait:      func() { c.Wait() },
				Broadcast: func() { c.Broadcast() },
			})
		})
	}
}

type condwrap struct {
	L         *sync.Mutex
	Wait      func()
	Broadcast func()
	Signal    func()
}

func skipWithoutDebug(t *testing.T) {
	if !debug {
		t.Skip("can run only with 'debug' build tag")
	}
}
