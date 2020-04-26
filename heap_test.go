package xsync

import (
	"reflect"
	"sort"
	"testing"
)

func TestHeapSwap(t *testing.T) {
	for _, test := range []struct {
		name string
		in   []uint32
		exp  []uint32
	}{
		{
			in: []uint32{1, 2, 10, 4},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := makeHeap(test.in...)
			verify(t, h)
		})
		t.Run(test.name, func(t *testing.T) {
			h := makeHeap(test.in...)
			g := h.Pop()
			if g.pos != -1 {
				t.Fatal("Pop() did not reset wg.pos")
			}
			verify(t, h)
		})
	}
}

func TestHeapPop(t *testing.T) {
	for _, test := range []struct {
		name string
		in   []uint32
		exp  []uint32
	}{
		{
			in:  []uint32{3, 1, 2, 5, 7, 10},
			exp: []uint32{1, 2, 3, 5, 7, 10},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := makeHeap(test.in...)
			act := tickets(h)
			if exp := test.exp; !reflect.DeepEqual(act, exp) {
				t.Fatalf("unexpected Pop() sequence: %v; want %v", act, exp)
			}
		})
	}
}

func TestHeapSort(t *testing.T) {
	for _, test := range []struct {
		name string
		in   []uint32
	}{
		{
			in: []uint32{1, 2, 3},
		},
		{
			in: []uint32{4, 2, 3, 1, 5},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := makeHeap(test.in...)
			act := tickets(h)

			exp := test.in
			sort.Slice(exp, func(i, j int) bool {
				return exp[i] < exp[j]
			})

			if !reflect.DeepEqual(act, exp) {
				t.Fatalf("heapsort failed: %v; want %v", act, exp)
			}
		})
	}
}

func makeHeap(xs ...uint32) *heap {
	data := make([]*wg, len(xs))
	for i, x := range xs {
		data[i] = &wg{
			t:   x,
			pos: i,
		}

	}
	h := &heap{data: data}
	heapify(h)
	return h
}

func verify(t *testing.T, h *heap) {
	for exp, wg := range h.data {
		if act := wg.pos; act != exp {
			t.Fatalf("unexpected wg.pos: %d; want %d", act, exp)
		}
	}
}

func heapify(h *heap) {
	p := parent(len(h.data) - 1) // Last parent node.
	for ; p >= 0; p-- {
		h.siftDown(p)
	}
}

func ticketsSorted(h *heap) []uint32 {
	ret := make([]uint32, len(h.data))
	for i := len(h.data) - 1; i >= 0; i-- {
		g := h.Pop()
		ret[i] = g.t
	}
	return ret
}

func tickets(h *heap) []uint32 {
	ret := make([]uint32, 0, len(h.data))
	for h.Size() > 0 {
		g := h.Pop()
		ret = append(ret, g.t)
	}
	return ret
}
