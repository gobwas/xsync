package xsync

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestWorkerGroupCloseError(t *testing.T) {
	p := &WorkerGroup{
		SizeLimit: 1,
		QueueSize: 1,
	}
	p.Close()
	if err := p.Exec(Demand{}, nil); err != ErrClosed {
		t.Errorf(
			"unexpected Exec() result: %v; want %v",
			err, ErrClosed,
		)
	}
}

func TestWorkerGroupClose(t *testing.T) {
	for _, test := range []struct {
		name  string
		tasks int
		size  int
	}{
		{
			name:  "t1/s1",
			tasks: 1,
			size:  1,
		},
		{
			name:  "t5/s1",
			tasks: 5,
			size:  1,
		},
		{
			name:  "t5/s10",
			tasks: 5,
			size:  10,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := &WorkerGroup{
				SizeLimit: test.size,
				QueueSize: test.tasks,
			}

			release := make(chan struct{})
			for i := 0; i < test.tasks; i++ {
				err := p.Exec(Demand{}, TaskFunc(func(*WorkerContext) {
					<-release
				}))
				if err != nil {
					t.Fatalf("Exec() error: %v", err)
				}
			}

			closed := make(chan struct{})
			go func() {
				p.Close()
				close(closed)
			}()
			select {
			case <-closed:
				t.Fatalf("Close() returned before tasks are done")
			case <-time.After(time.Millisecond * 50):
			}

			close(release)

			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatalf("Close() did not return after 1s")
			}
		})
	}
}

func TestWorkerGroupFlush(t *testing.T) {
	skipWithoutDebug(t)

	for _, test := range []struct {
		name string
		wg   *WorkerGroup

		taskDuration time.Duration
	}{
		{
			wg: &WorkerGroup{
				SizeLimit: 5,
			},
			taskDuration: 100 * time.Millisecond,
		},
		{
			wg: &WorkerGroup{
				SizeLimit: 10,
			},
			taskDuration: 100 * time.Millisecond,
		},
		{
			wg: &WorkerGroup{
				SizeLimit: 10,
				QueueSize: 10,
				FetchSize: 10,
			},
			taskDuration: 100 * time.Millisecond,
		},
		{
			wg: &WorkerGroup{
				SizeLimit: 10,
				QueueSize: 10,
				FetchSize: 1,
			},
			taskDuration: 100 * time.Millisecond,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wg := test.wg
			waitScheduled := make(chan struct{})
			wg.hookFlushScheduled = func() {
				close(waitScheduled)
			}
			defer wg.Close()

			var (
				done    = make(chan struct{})
				release = make(chan struct{})
			)
			for i := 0; i < 5; i++ {
				wg.Exec(Demand{}, TaskFunc(func(ctx *WorkerContext) {
					<-release
				}))
			}

			go func() {
				wg.Flush(context.Background())
				close(done)
			}()
			<-waitScheduled

			go func() {
				for {
					select {
					case <-done:
					default:
						err := wg.Exec(Demand{}, TaskFunc(func(ctx *WorkerContext) {
							time.Sleep(test.taskDuration)
						}))
						if err != nil {
							return
						}
					}
				}
			}()

			for i := 0; i < 5; i++ {
				select {
				case <-done:
					t.Fatalf("Flush() returned before tasks are done")
				case <-time.After(50 * time.Millisecond):
					release <- struct{}{}
				}
			}
			const timeout = time.Second
			select {
			case <-done:
			case <-time.After(timeout):
				t.Fatalf("Flush() did not return after %s", timeout)
			}
		})
	}
}

func BenchmarkWorkerGroupExec(b *testing.B) {
	type workerGroupWrapper struct {
		Exec  func(task func())
		Close func()
	}
	for _, test := range []struct {
		name string
		size []int
		wg   func(size int) *workerGroupWrapper
	}{
		{
			name: "naive",
			size: []int{1, 4, 8, 100},
			wg: func(size int) *workerGroupWrapper {
				n := naiveWorkerGroup{
					Size: size,
				}
				return &workerGroupWrapper{
					Exec: func(task func()) {
						n.Exec(task)
					},
					Close: func() {
						n.Close()
					},
				}
			},
		},
		{
			name: "real",
			size: []int{1, 4, 8, 100},
			wg: func(size int) *workerGroupWrapper {
				n := WorkerGroup{
					SizeLimit: size,
				}
				return &workerGroupWrapper{
					Exec: func(task func()) {
						n.Exec(Demand{}, TaskFunc(func(*WorkerContext) {
							task()
						}))
					},
					Close: func() {
						n.Close()
					},
				}
			},
		},
	} {
		for _, size := range test.size {
			name := fmt.Sprintf("%s/%d", test.name, size)
			b.Run(name+"s", func(b *testing.B) {
				wg := test.wg(size)
				defer wg.Close()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					wg.Exec(func() {
						//
					})
				}
			})
			b.Run(name+"p", func(b *testing.B) {
				wg := test.wg(size)
				defer wg.Close()
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						wg.Exec(func() {
							//
						})
					}
				})
			})
		}
	}

}

type naiveWorkerGroup struct {
	Size int

	once sync.Once
	sem  chan struct{}
	exit chan struct{}
	work chan func()
}

func (n *naiveWorkerGroup) init() {
	n.once.Do(func() {
		n.sem = make(chan struct{}, max(1, n.Size))
		n.work = make(chan func())
		n.exit = make(chan struct{})
	})
}

func (n *naiveWorkerGroup) Exec(fn func()) {
	n.init()
repeat:
	select {
	case n.work <- fn:
	case n.sem <- struct{}{}:
		go n.worker()
		goto repeat
	case <-n.exit:
		panic("Exec() on closed naiveWorkerGroup")
	}
}

func (n *naiveWorkerGroup) Close() {
	n.init()
	close(n.exit)
	for i := 0; i < cap(n.sem); i++ {
		n.sem <- struct{}{}
	}
}

func (n *naiveWorkerGroup) worker() {
	defer func() {
		<-n.sem
	}()
	for {
		select {
		case fn := <-n.work:
			fn()
		case <-n.exit:
			return
		}
	}
}
