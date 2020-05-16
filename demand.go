package xsync

import (
	"context"
)

//
type Priority interface {
	// Compare compares itself with given priority.
	//
	// The result must be zero if both priorities are equal, less than zero if
	// given priority is higher, and greater than zero if given priority is
	// lower.
	//
	// Note that given argument might be nil.
	// When using with the WorkerGroup argument might also be of LowestPriority
	// type, which must be treated as always lower priority than application's.
	Compare(Priority) int
}

// BytePriority is a priority specified by byte.
type BytePriority byte

// Compare implements Priority interface.
// If given priority is not BytePriority, it always returns 1.
func (b BytePriority) Compare(x Priority) int {
	v, ok := x.(BytePriority)
	if !ok {
		// TODO?
		return 1
	}
	return int(b) - int(v)
}

// LowestPriority is a priority which must be treated as lower than any other
// priority.
//
// Integer value it holds is used to compare instances which are both of
// LowestPriority type.
type LowestPriority int64

// Compare implements Priority interface.
func (p LowestPriority) Compare(x Priority) int {
	v, ok := x.(LowestPriority)
	if !ok {
		return -1
	}
	d := p - v
	if d < 0 {
		return -1
	}
	if d > 0 {
		return +1
	}
	return 0
}

func comparePriority(p1, p2 Priority) (c int) {
	switch {
	case p1 != nil:
		c = p1.Compare(p2)

	case p2 != nil:
		c = p2.Compare(p1)
		c = invert(c)
	}
	return c
}

// Demand represents a caller's demand on condition.
type Demand struct {
	// Priority represends demand priority.
	Priority Priority

	// Cancel is a demand cancelation channel.
	Cancel <-chan struct{}
}

type priorityContextKey struct{}

// WithPriority returns context with associated priority p.
func WithPriority(ctx context.Context, p Priority) context.Context {
	return context.WithValue(ctx, priorityContextKey{}, p)
}

// ContextPriority returns a priority associated with given context.
func ContextPriority(ctx context.Context) Priority {
	p, _ := ctx.Value(priorityContextKey{}).(Priority)
	return p
}

// ContextDemand is a helper function which constructs Demand structure which
// Cancel field is set to ctx.Done(), and Priority is set to result of
// ContextPriority(ctx).
func ContextDemand(ctx context.Context) Demand {
	return Demand{
		Cancel:   ctx.Done(),
		Priority: ContextPriority(ctx),
	}
}
