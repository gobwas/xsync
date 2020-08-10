package container

import (
	"reflect"
	"sort"
	"testing"
)

func TestHeapRemoveLast(t *testing.T) {
	var h genericHeap
	x0 := &genericHeapItem{
		value: 0,
	}
	x1 := &genericHeapItem{
		value: 1,
	}
	h.Push(x0)
	h.Push(x1)
	h.Remove(x0)
	if act, exp := h.Pop().value, x1.value; act != exp {
		t.Fatal("unexpected item")
	}
}

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
			x := h.Pop()
			if x.pos != -1 {
				t.Fatal("Pop() did not reset item.pos")
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
			exp: []uint32{10, 7, 5, 3, 2, 1},
		},
		{
			in:  []uint32{3},
			exp: []uint32{3},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := makeHeap(test.in...)
			act := values(h)
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
			act := values(h)

			exp := test.in
			sort.Slice(exp, func(i, j int) bool {
				return exp[i] > exp[j]
			})

			if !reflect.DeepEqual(act, exp) {
				t.Fatalf("heapsort failed: %v; want %v", act, exp)
			}
		})
	}
}

func makeHeap(xs ...uint32) *genericHeap {
	data := make([]*genericHeapItem, len(xs))
	for i, x := range xs {
		data[i] = &genericHeapItem{
			pos:   i,
			value: x,
		}
	}
	h := &genericHeap{data: data}
	heapify(h)
	return h
}

func verify(t *testing.T, h *genericHeap) {
	for exp, item := range h.data {
		if act := item.pos; act != exp {
			t.Fatalf("unexpected item.pos: %d; want %d", act, exp)
		}
	}
}

func heapify(h *genericHeap) {
	p := h.parent(len(h.data) - 1) // Last parent node.
	for ; p >= 0; p-- {
		h.siftDown(p)
	}
}

func valuesSorted(h *genericHeap) []uint32 {
	ret := make([]uint32, len(h.data))
	for i := len(h.data) - 1; i >= 0; i-- {
		item := h.Pop()
		ret[i] = item.value
	}
	return ret
}

func values(h *genericHeap) []uint32 {
	ret := make([]uint32, 0, len(h.data))
	for h.Size() > 0 {
		item := h.Pop()
		ret = append(ret, item.value)
	}
	return ret
}
