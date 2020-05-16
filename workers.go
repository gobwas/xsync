package xsync

import (
	container "container/list"
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/xsync/internal/buildtags"
)

// WorkerContext represents worker goroutine context.
type WorkerContext struct {
	context.Context
	id uint32
}

// ID returns worker identifier within its group.
// Returned id is always an integer that is less than worker group size.
//
// NOTE: ID might be reused after worker exit as idle.
func (c *WorkerContext) ID() uint32 {
	return c.id
}

// Task is the interface that holds task implementaion.
type Task interface {
	// Exec executes the task.
	// Given context holds the worker goroutine related info. Context might be
	// canceled when worker group is closing.
	Exec(*WorkerContext)
}

// TaskFunc is an adapter to allow the use of ordinary functions as Task.
type TaskFunc func(*WorkerContext)

// Exec implements Task.
func (f TaskFunc) Exec(ctx *WorkerContext) {
	f(ctx)
}

// WorkerGroup contains options and logic of managing worker goroutines and
// sharing work between them.
type WorkerGroup struct {
	// QueueSize specifies the size of the internal tasks queue. Note that
	// workers fetch tasks from the queue in accordance with task priority (if
	// any was given).
	//
	// The greater queue size the more tasks with high priority will be
	// executed at first. The less queue size, the less difference in execution
	// order between tasks with different priorities.
	//
	// Note that FetchSize field also affects workers behaviour.
	QueueSize int

	// FetchSize specifies how many tasks will be pulled from the queue per
	// each scheduling cycle.
	//
	// The smaller FetchSize the higher starvation rate for the low priority
	// tasks. Contrariwise, when FetchSize is equal to QueueSize, then all
	// previously scheduled tasks will be fetched from the queue; that is,
	// queue will be drained.
	//
	// FetchSize must not be greater than QueueSize.
	FetchSize int

	// SizeLimit specifies the capacity of the worker group.
	// If SizeLimit is zero then worker group will contain one worker.
	SizeLimit int

	// IdleLimit specifies the maximum number of idle workers.
	// When set, IdleTimeout must also be set.
	// If IdleLimit is zero then no idle limit is used.
	IdleLimit int

	// IdleTimeout specifies the duration after which worker is considered
	// idle.
	IdleTimeout time.Duration

	// OnStart is an optional callback that will be called right after worker
	// goroutine has started.
	OnStart func(*WorkerContext)

	// OnComplete is an optional callback that will be called right before
	// worker goroutine complete.
	OnComplete func(*WorkerContext)

	initOnce  sync.Once
	closeOnce sync.Once

	start time.Time // Used for monotonic clocks.

	sem        chan struct{}
	work       chan Task
	manageDone chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc

	queue queue
	drain uint32

	mu sync.RWMutex
	// workers is a list of allocated workers.
	// Note that it may contain workers that already stopped.
	workers *container.List

	id idPool

	// These hooks are called only if debug buildtag passed.
	hookFlushScheduled func()
}

func (w *WorkerGroup) init() {
	w.initOnce.Do(func() {
		w.start = time.Now()

		w.queue.init(max(w.QueueSize, 1))

		w.manageDone = make(chan struct{})
		w.work = make(chan Task)
		w.sem = make(chan struct{}, max(w.SizeLimit, 1))
		w.ctx, w.cancel = context.WithCancel(context.Background())

		w.workers = container.New()

		// Start task manager goroutine.
		// It receives tasks from the queue and puts them into the work
		// channel starting new workers if needed and possible.
		go w.manage()
	})
}

func (w *WorkerGroup) manage() {
	defer close(w.manageDone)

	// Prepare tasks buffer where tasks would be moved to from the queue.
	var (
		full = make([]Task, max(w.QueueSize, 1))
		part = full[:max(w.FetchSize, 1)]
	)

	// We are trying to receive as many tasks from queue as possible to reduce
	// number of synchronizations with queue. That is, lock w.queue's mutex
	// less times.
	//
	// This behavior also helps a bit to prevent tasks starvation, when lower
	// priority tasks executed less frequently due to the high pressure of
	// tasks with higher priority (this is not works when FetchSize is
	// specified).
	for {
		var tasks []Task
		if atomic.CompareAndSwapUint32(&w.drain, 1, 0) {
			// Need to fetch all the tasks from queue.
			tasks = full
		} else {
			tasks = part
		}
		n, err := w.queue.recvTo(tasks, nil)
		if err == ErrClosed {
			// Close w.work to signal all started workers that no more work
			// will be feeded.
			close(w.work)
			return
		}
		if err != nil {
			panic(fmt.Sprintf("xsync: workers: unexpected error: %v", err))
		}
		for i := 0; i < n; i++ {
			var direct chan<- Task
			for {
				select {
				case direct <- tasks[i]:
				case w.work <- tasks[i]:
				default:
					select {
					case direct <- tasks[i]:
					case w.work <- tasks[i]:
					case w.sem <- struct{}{}:
						worker := w.startWorker()
						direct = worker.direct
						continue
					}
				}
				break
			}
		}
	}
}

func (w *WorkerGroup) startWorker() *worker {
	worker := &worker{
		common: w.work,
		direct: make(chan Task, 1),
		done:   make(chan struct{}),
	}

	// NOTE: Need to do slow path in caller's goroutine to get rid of spurious
	// workers start. Moreover we do Gosched() at the end of the function to
	// give the worker goroutine chance to get its processor.
	defer runtime.Gosched()

	w.mu.Lock()
	el := w.workers.PushBack(worker)
	n := w.workers.Len()
	w.mu.Unlock()

	// NOTE: w.workers can contain stopped but not yet removed workers; but its
	// not a problem for counting such workers while making decision on idle
	// timer below. We always have "non-preemtible" goroutines there, so if the
	// workers count is greater or equal than IdleLimit it would be so even
	// after removal of stopped worker from the list.
	var idle time.Duration
	if limit := w.IdleLimit; limit > 0 && n > limit {
		idle = w.IdleTimeout
	}

	id := w.id.acquire()
	ctx, cancel := context.WithCancel(w.ctx)
	wctx := &WorkerContext{
		Context: ctx,
		id:      id,
	}
	go func() {
		if fn := w.OnStart; fn != nil {
			fn(wctx)
		}

		worker.work(wctx, idle)

		if fn := w.OnComplete; fn != nil {
			fn(wctx)
		}

		cancel()
		w.id.release(id)

		w.mu.Lock()
		if w.workers != nil {
			w.workers.Remove(el)
		}
		w.mu.Unlock()

		<-w.sem
	}()

	return worker
}

type worker struct {
	common <-chan Task
	direct chan Task
	done   chan struct{}
	idle   time.Timer
}

func (w *worker) work(ctx *WorkerContext, idle time.Duration) {
	defer func() {
		cork(ctx, w.direct)
		close(w.done)
	}()
	var (
		timer   *time.Timer
		timeout <-chan time.Time
	)
	if idle > 0 {
		timer = time.NewTimer(idle)
		timeout = timer.C
	}
	for {
		var (
			t  Task
			ok bool
		)
		select {
		case t = <-w.direct:
		case t, ok = <-w.common:
			if !ok {
				return
			}
		case <-timeout:
			return
		}

		t.Exec(ctx)

		if timer != nil {
			if !timer.Stop() {
				<-timeout
			}
			timer.Reset(idle)
		}
	}
}

// Close terminates all spawned goroutines.
// It returns when all goroutines and all tasks scheduled before are done.
func (w *WorkerGroup) Close() {
	_ = w.close(nil)
}

func (w *WorkerGroup) Shutdown(ctx context.Context) error {
	return w.close(ctx.Done())
}

func (w *WorkerGroup) close(cancel <-chan struct{}) (err error) {
	w.init()
	w.closeOnce.Do(func() {
		// Close of queue leads to close of w.work, which in turn leads to
		// worker exit. So we don't need additional channels like manageDone.
		w.queue.close()

		// Cancel the parent context of all workers.
		w.cancel()

		// Need to wait for manager goroutine exit, to not compete with it in
		// accessing the workers list.
		select {
		case <-w.manageDone:
		case <-cancel:
			err = ErrCanceled
			return
		}

		w.mu.Lock()
		workers := w.workers
		w.workers = nil
		w.mu.Unlock()

		for el := workers.Front(); el != nil; el = el.Next() {
			w := el.Value.(*worker)
			select {
			case <-w.done:
			case <-cancel:
				err = ErrCanceled
				return
			}
		}
	})
	return err
}

// Exec makes t to be executed in one of the running workers.
func (w *WorkerGroup) Exec(d Demand, t Task) error {
	w.init()
	return w.queue.send(d, t)
}

// Flush waits for completion of all task successfully scheduled before.
//
// Note that Flush() call leads to one-time full queue fetch inside group.
// That is, it affects the priority of execution if w.FetchSize was set (and
// acts like w.FetchSize was not set).
func (w *WorkerGroup) Flush(ctx context.Context) error {
	w.init()
	return w.wait(ctx.Done())
}

// Barrier waits for completion of all currently running tasks.
//
// That is, having two workers in the group and three tasks T1, T2 and T3
// scheduled and T1 and T2 executing, call to Barrier() will block until T1 and
// T2 are done. It doesn't guarantee that T3 is done as well. To be sure that
// all tasks in the queue are done, use Flush().
func (w *WorkerGroup) Barrier(ctx context.Context) error {
	return w.barrier(ctx.Done())
}

func (w *WorkerGroup) monotonic() int64 {
	return int64(time.Since(w.start))
}

func (w *WorkerGroup) wait(cancel <-chan struct{}) error {
	ch := make(chan struct{}, 1)
	// Schedule task with lowest priority to be sure that it will be executed
	// after previously scheduled normal tasks.
	err := w.queue.send(
		Demand{
			Priority: LowestPriority(w.monotonic()),
			Cancel:   cancel,
		},
		TaskFunc(func(*WorkerContext) {
			ch <- struct{}{}
		}),
	)
	if err != nil {
		return err
	}

	// Signal manager goroutine that it must drain queue.
	atomic.StoreUint32(&w.drain, 1)

	if buildtags.Debug {
		if hook := w.hookFlushScheduled; hook != nil {
			hook()
		}
	}
	select {
	case <-ch:
	case <-cancel:
		return ErrCanceled
	}

	// Ensure all tasks before ours are done.
	return w.barrier(cancel)
}

func (w *WorkerGroup) barrier(cancel <-chan struct{}) error {
	ch := make(chan struct{}, 1)
	n, err := w.broadcast(TaskFunc(func(*WorkerContext) {
		select {
		case ch <- struct{}{}:
		case <-cancel:
		}
	}), cancel)
	for err == nil && n > 0 {
		select {
		case <-ch:
			n--
		case <-cancel:
			err = ErrCanceled
		}
	}
	return err
}

func (w *WorkerGroup) broadcast(t Task, cancel <-chan struct{}) (n int, err error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for el := w.workers.Front(); el != nil; el = el.Next() {
		w := el.Value.(*worker)
		select {
		case w.direct <- t:
			n++
		case <-w.done:
			// That is fine. Less things to do.
		case <-cancel:
			err = ErrCanceled
			return
		}
	}
	return n, err
}

func cork(ctx *WorkerContext, ch chan Task) {
	for i, c := 0, cap(ch); i != c; {
	corking:
		for i = 0; i < c; i++ {
			select {
			case ch <- nil:
			default:
				break corking
			}
		}
		for j := 0; i != c && j < c; j++ {
			task := <-ch
			if task != nil {
				task.Exec(ctx)
			}
		}
	}
}

type idPool struct {
	mu   sync.Mutex
	free []uint32
	next uint32
}

func (p *idPool) acquire() (id uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n := len(p.free); n > 0 {
		id = p.free[0]
		p.free[0] = p.free[n-1]
		p.free = p.free[:n-1]
	} else {
		id = p.next
		p.next++
	}
	return id
}

func (p *idPool) release(id uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.free = append(p.free, id)
}
