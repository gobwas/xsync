package xsync

import (
	"sync/atomic"
)

const (
	maxU32 = 0x3fffffff // Lower 30 bits for value.
	genU32 = 0xc0000000 // Higher 2 bits for generation.

	minusOne = ^uint32(0)
)

// dcounter represents a counter that works with two numbers A and B.
// Note that A and B has little difference in how changes are handled.
// If one number may not be changed during operation, this number must be the
// B-counter. That is, dcounter is optimized for cases when the A-counter is
// always changed, but B-counter may stay untouched.
//
// Note that biggest positive value each part can take is 2^31 - 1, that is,
// only 30 bits are used for storing value.
//
// Overflow in both directions (maxU32+1 or 0-1) are allowed and tracked by
// higher two bits called "generation".
type dcounter struct {
	bits uint64
}

// Add atomically adds two deltas to the left and the right parts and returns
// the new values.
//
// To subtract a signed positive constant values a, b from counter, do
// add(^uint32(a-1), ^uint32(b-1)).
//
//go:norace
func (d *dcounter) Add(delta1, delta2 uint32) (uint32, uint32) {
	var (
		d1   = value(delta1)
		d2   = value(delta2)
		neg1 = isNegative(d1)
		neg2 = isNegative(d2)
	)
	for {
		bits := d.bits
		b1, b2 := split(bits)
		r1 := sum(b1, d1, neg1)
		r2 := sum(b2, d2, neg2)
		if atomic.CompareAndSwapUint64(&d.bits, bits, join(r1, r2)) {
			return r1, r2
		}
	}
}

// Store atomically stores both numbers in d.
func (d *dcounter) Store(a, b uint32) {
	atomic.StoreUint64(&d.bits, join(a, b))
}

// Load atomically loads both numbers.
func (d *dcounter) Load() (a, b uint32) {
	bits := atomic.LoadUint64(&d.bits)
	return split(bits)
}

// cmpGen is a table containing all possible generation comparison states.
var cmpGen [16]int

func init() {
	for x := uint32(0); x < 1<<4; x++ {
		const mask = 0x3
		g1 := x >> 2
		g2 := x & mask
		var c int
		switch {
		case g1 == g2:
			c = 0
		case (g1+1)&mask == g2:
			c = -1
		case (g1-1)&mask == g2:
			c = +1
		default:
			c = -2
		}
		cmpGen[x] = c
	}
}

// Must monotonically grow.
func comparebits(t1, t2 uint32) int {
	g1 := generation(t1)
	g2 := generation(t2)
	c := cmpGen[g1<<2|g2]
	if c == -2 {
		panic("xsync: distance between ticket generations is too big")
	}
	if c == 0 {
		// We assume here that both values are from same generation.
		c = int(value(t1)) - int(value(t2))
	}
	return c
}

func equalbits(t1, t2 uint32) bool {
	return comparebits(t1, t2) == 0
}

func isNegative(b uint32) bool {
	const mask = 0x20000000
	return b&mask != 0
}

func value(b uint32) uint32 {
	return b & maxU32
}

func generation(b uint32) uint32 {
	return (b & genU32) >> 30
}

func makebits(gen uint32, value uint32) uint32 {
	return gen<<30 | value
}

func sum(bits uint32, delta uint32, isNeg bool) uint32 {
	v := value(bits)
	r := (v + delta) & maxU32
	g := generation(bits)
	if isNeg && r > v {
		g--
	}
	if !isNeg && r < v {
		g++
	}
	return makebits(g, r)
}

func join(a, b uint32) uint64 {
	return (uint64(a) << 32) | uint64(b)
}

func split(v uint64) (a, b uint32) {
	return uint32(v >> 32), uint32(v)
}
