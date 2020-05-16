package container

type genericHeapItem struct {
	pos   int
	value uint32
}

func (g genericHeapItem) less(b genericHeapItem) bool {
	return g.value < b.value
}
