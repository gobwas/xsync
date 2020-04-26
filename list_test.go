package xsync

import (
	"testing"
)

// TestListDelayedNotify tests that list releases goroutines in the order of
// the arriving with Wait() call. This is not completely fair but see the list
// docs for more ideas.
func TestListDelayedNotify(t *testing.T) {
	skipWithoutDebug(t)

	for _, test := range []struct {
		name string
		wait []int
	}{
		{
			wait: []int{0},
		},
		{
			wait: []int{1, 0},
		},
		{
			wait: []int{2, 1, 0},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			l := new(list)

			tickets := make([]uint32, len(test.wait))
			for i := range tickets {
				tickets[i] = l.Add()
			}
			for range test.wait {
				l.Notify()
			}
			// Make tickets arrived.
			for _, i := range test.wait {
				l.Wait(tickets[i], Demand{})
			}
		})
	}
}
