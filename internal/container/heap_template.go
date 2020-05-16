package container

type genericHeap struct {
	data []genericHeapItem
}

var genericHeapZeroItem genericHeapItem

func (h *genericHeap) Push(x genericHeapItem) {
	i := len(h.data)
	h.data = append(h.data, x)
	x.pos = i
	h.siftUp(i)
}

func (h *genericHeap) Pop() genericHeapItem {
	return h.remove(0)
}

func (h *genericHeap) Max() genericHeapItem {
	if len(h.data) > 0 {
		return h.data[0]
	}
	return genericHeapZeroItem
}

func (h *genericHeap) Remove(x genericHeapItem) bool {
	if h.data[x.pos] == x {
		h.remove(x.pos)
		return true
	}
	return false
}

func (h *genericHeap) Size() int {
	return len(h.data)
}

func (h *genericHeap) Reserve(n int) {
	m := len(h.data)
	if m < n {
		d := make([]genericHeapItem, m, n)
		copy(d, h.data)
		h.data = d
	}
}

func (h *genericHeap) IsFull() bool {
	return len(h.data) == cap(h.data)
}

func (h *genericHeap) IsEmpty() bool {
	return len(h.data) == 0
}

func (h *genericHeap) remove(i int) genericHeapItem {
	n := h.Size()
	if n == 0 {
		return genericHeapZeroItem
	}

	x := h.data[i]
	h.swap(i, n-1)
	x.pos = -1
	h.data[n-1] = genericHeapZeroItem
	h.data = h.data[:n-1]

	if p := h.parent(i); p < len(h.data) && h.data[p].less(h.data[i]) {
		h.siftUp(i)
	} else {
		h.siftDown(i)
	}

	return x
}

func (h *genericHeap) swap(i, j int) {
	//log.Printf("swap #%d[%v]@%d <-> #%d[%v]@%d", i, h.data[i].t, h.data[i].pos, j, h.data[j].t, h.data[j].pos)
	h.data[i], h.data[j] = h.data[j], h.data[i]
	h.data[i].pos = i
	h.data[j].pos = j
	//log.Printf("now: #%d[%v]@%d", i, h.data[i].t, h.data[i].pos)
	//log.Printf("now: #%d[%v]@%d", j, h.data[j].t, h.data[j].pos)
}

func (h *genericHeap) siftUp(i int) {
	for i > 0 {
		p := h.parent(i)
		if !h.data[p].less(h.data[i]) {
			return
		}
		h.swap(p, i)
		i = p
	}
}

func (h *genericHeap) siftDown(i int) {
	for {
		max := i
		i1, i2 := h.children(i)
		if i1 < len(h.data) && h.data[max].less(h.data[i1]) {
			max = i1
		}
		if i2 < len(h.data) && h.data[max].less(h.data[i2]) {
			max = i2
		}
		if max == i {
			break
		}
		h.swap(i, max)
		i = max
	}
}

func (h *genericHeap) parent(x int) int {
	return (x - 1) / 2
}

func (h *genericHeap) children(x int) (int, int) {
	return 2*x + 1, 2*x + 2
}
