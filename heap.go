package xsync

type heap struct {
	data []*wg
}

func (h *heap) Push(x *wg) {
	i := len(h.data)
	h.data = append(h.data, x)
	x.pos = i
	h.siftUp(i)
}

func (h *heap) Pop() *wg {
	return h.remove(0)
}

func (h *heap) Max() *wg {
	if len(h.data) > 0 {
		return h.data[0]
	}
	return nil
}

func (h *heap) Remove(x *wg) bool {
	return h.remove(x.pos) == x
}

func (h *heap) Size() int {
	return len(h.data)
}

func (h *heap) remove(i int) *wg {
	n := h.Size()
	if n == 0 {
		return nil
	}

	x := h.data[i]
	h.swap(i, n-1)
	x.pos = -1
	h.data[n-1] = nil
	h.data = h.data[:n-1]

	if p := parent(i); p < len(h.data) && less(h.data[p], h.data[i]) {
		h.siftUp(i)
	} else {
		h.siftDown(i)
	}

	return x
}

func (h *heap) swap(i, j int) {
	//log.Printf("swap #%d[%v]@%d <-> #%d[%v]@%d", i, h.data[i].t, h.data[i].pos, j, h.data[j].t, h.data[j].pos)
	h.data[i], h.data[j] = h.data[j], h.data[i]
	h.data[i].pos = i
	h.data[j].pos = j
	//log.Printf("now: #%d[%v]@%d", i, h.data[i].t, h.data[i].pos)
	//log.Printf("now: #%d[%v]@%d", j, h.data[j].t, h.data[j].pos)
}

func (h *heap) siftUp(i int) {
	for i > 0 {
		p := parent(i)
		if !less(h.data[p], h.data[i]) {
			return
		}
		h.swap(p, i)
		i = p
	}
}

func (h *heap) siftDown(i int) {
	for {
		max := i
		i1, i2 := children(i)
		if i1 < len(h.data) && less(h.data[max], h.data[i1]) {
			max = i1
		}
		if i2 < len(h.data) && less(h.data[max], h.data[i2]) {
			max = i2
		}
		if max == i {
			break
		}
		h.swap(i, max)
		i = max
	}
}

func less(a, b *wg) bool {
	return compare(a, b) < 0
}

func parent(x int) int {
	return (x - 1) / 2
}

func children(x int) (int, int) {
	return 2*x + 1, 2*x + 2
}
