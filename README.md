# xsync

[![GoDoc][godoc-image]][godoc-url]
[![Travis][travis-image]][travis-url]

> Priortizable and cancellable synchronization primitives in Go.

# Usage

## xsync.WorkerGroup

```go
package main

import (
	"sync"

	"github.com/gobwas/xsync"
)

func main() {
	var wg xsync.WorkerGroup
	wg.Exec(
		xsync.Demand{
			Priority: xsync.BytePriority(42),
		},
		xsync.TaskFunc(func(ctx *xsync.WorkerContext) {

		}),
	})
	wg.Close()
}
```

## xsync.Cond

```go
package main

import (
	"sync"

	"github.com/gobwas/xsync"
)

func main() {
	var mu sync.Mutex
	cond := xsync.Cond{
		L: &mu,
	}
	mu.Lock()
	for !condition {
		cond.Wait(xsync.Demand{
			Priority: xsync.BytePriority(42),
		})
	}
	// action on condition.
	mu.Unlock()
}
```

## examples

For more examples please take a look into _examples_ folder.

# Why?

The idea of this package appeared year ago or something when I was playing with
high-performant concurrent programming. The main intention of the package is to
provide an ability to select which kind of task should be performed when
execution resources are limited.

# Status

This package is one of those projects that might be developed but then not then
released anywhen. Fortunately I found time and motivation to complete it and
share with the community.

Current API is not fixed, thus there is no `v1.0.0` tag yet. My main concerns
are related to the `Demand` interface which I don't like much. Maybe I will
change it over time.

# Performance

Goroutine blocking mechanics are built over channels. Thus, it can't be more
efficient than standard libarary's `sync` package. There are few benchmarks in
tests, so feel free to run them.

[godoc-image]:  https://godoc.org/github.com/gobwas/xsync?status.svg
[godoc-url]:    https://godoc.org/github.com/gobwas/xsync
[travis-image]: https://travis-ci.org/gobwas/xsync.svg?branch=master
[travis-url]:   https://travis-ci.org/gobwas/xsync
