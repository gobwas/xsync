package xsync

type Priority interface {
	Compare(Priority) int
}

// Demand represents a caller's demand on condition.
type Demand struct {
	// Priority represends demand priority.
	Priority uint

	// Custom is a custom object capable to represent demand priority.
	//
	// If Custom priority is used then it must be used for all interaction per
	// synchronization primitive instance.
	Custom Priority

	// Cancel is a demand cancelation channel.
	Cancel <-chan struct{}

	// Slice of channels to select from while waiting.
	//Select []interface{}
}

func (d Demand) compare(x Demand) int {
	switch {
	case d.Custom != nil && x.Custom != nil:
		return d.Custom.Compare(x.Custom)
	case d.Custom == nil && x.Custom == nil:
		return int(d.Priority) - int(x.Priority)
	}
	panic("xsync: use of different priority types")
}
