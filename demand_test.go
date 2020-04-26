package xsync

import (
	"sync"
	"testing"
)

func BenchmarkDemandOptions(b *testing.B) {
	var mu sync.Mutex
	cond := Cond{
		L: &mu,
	}
	ch := make(chan struct{})
	close(ch)
	for i := 0; i < b.N; i++ {
		mu.Lock()
		cond.Wait(Demand{
			Priority: i,
			Cancel:   ch,
		})
		mu.Unlock()
	}
}
