package main

import (
	"fmt"
	"runtime"

	"github.com/gobwas/xsync"
)

func main() {
	data := make([]int, 8)
	wg := xsync.WorkerGroup{
		SizeLimit: len(data),
		OnStart: func(ctx *xsync.WorkerContext) {
			runtime.LockOSThread()
			fmt.Println("started", ctx.ID())
			data[ctx.ID()] = 0
		},
		OnComplete: func(ctx *xsync.WorkerContext) {
			fmt.Println("completed", ctx.ID())
		},
	}
	for i := 0; i < 1<<20; i++ {
		wg.Exec(xsync.Demand{}, xsync.TaskFunc(func(ctx *xsync.WorkerContext) {
			data[ctx.ID()]++
		}))
	}
	wg.Close()
	for i, v := range data {
		fmt.Printf("#%d: %v\n", i, v)
	}
}
