package xsync

import (
	"reflect"
	"sync"
	"testing"
)

type TaskInt int

func (TaskInt) Exec(*WorkerContext) {}

var immediately = func() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

func TestQueue(t *testing.T) {
	for _, test := range []struct {
		name     string
		size     int
		rcvn     int
		send     []int
		priority []byte
		recv     [][]int
	}{
		{
			size:     1,
			rcvn:     1,
			send:     []int{1, 2, 3},
			priority: []byte{0, 0, 0},
			recv: [][]int{
				{1}, {2}, {3},
			},
		},
		{
			size:     1,
			rcvn:     1,
			send:     []int{1, 2, 3},
			priority: []byte{1, 2, 3},
			recv: [][]int{
				{1}, {2}, {3},
			},
		},
		{
			size:     2,
			rcvn:     1,
			send:     []int{1, 2, 3},
			priority: []byte{1, 2, 3},
			recv: [][]int{
				{2}, {3}, {1},
			},
		},
		{
			size:     3,
			rcvn:     1,
			send:     []int{1, 2, 3},
			priority: []byte{1, 2, 3},
			recv: [][]int{
				{3}, {2}, {1},
			},
		},
		{
			size:     3,
			rcvn:     4,
			send:     []int{1, 2, 3},
			priority: []byte{1, 2, 3},
			recv: [][]int{
				{3, 2, 1, -1},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var q queue
			q.init(test.size)

			var (
				sendDone = make(chan struct{})
				recvDone = make(chan struct{})

				mu   sync.Mutex
				cond sync.Cond
			)
			cond.L = &mu

			const (
				stateSend     = 0x1
				stateRecv     = 0x2
				stateSendDone = 0x4
			)

			state := stateSend
			go func() {
				defer close(sendDone)

				var i int
				for j := 0; i < len(test.send); j++ {
					mu.Lock()
					if state&stateSend == 0 {
						cond.Wait()
					}
					for ; i < len(test.send); i++ {
						x := test.send[i]
						p := test.priority[i]
						err := q.send(Demand{
							Priority: BytePriority(p),
							Cancel:   immediately,
						}, TaskInt(x))
						if err != nil {
							// No space in queue so let the receiving goroutine
							// receive tasks.
							break
						}
					}
					state = stateRecv
					mu.Unlock()
					cond.Signal()
				}
				mu.Lock()
				state = stateSendDone
				mu.Unlock()
			}()

			var act [][]int
			go func() {
				defer close(recvDone)

				ts := make([]Task, test.rcvn)
				for range test.recv {
					mu.Lock()
					if state&(stateRecv|stateSendDone) == 0 {
						cond.Wait()
					}
					n, err := q.recvTo(ts, nil)
					if err != nil {
						t.Fatal(err)
					}
					xs := make([]int, len(ts))
					for i := 0; i < n; i++ {
						xs[i] = int((ts[i].(TaskInt)))
					}
					for i := len(xs) - 1; i >= n; i-- {
						xs[i] = -1
					}
					act = append(act, xs)
					state &^= stateRecv
					state |= stateSend
					mu.Unlock()
					cond.Signal()
				}
			}()

			<-sendDone
			<-recvDone

			for i, exp := range test.recv {
				act := act[i]
				if !reflect.DeepEqual(act, exp) {
					t.Errorf("unexpected #%d recv: %v; want %v", i, act, exp)
				}
			}
		})
	}
}
