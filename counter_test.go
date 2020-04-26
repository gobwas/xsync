package xsync

import (
	"testing"
)

func TestCounterAdd(t *testing.T) {
	type pair struct {
		a uint32
		b uint32
	}
	for _, test := range []struct {
		name     string
		init     pair
		add      pair
		expValue pair
		expGen   pair
	}{
		// Increment cases.
		{
			name:     "increment a",
			init:     pair{0, 0},
			add:      pair{1, 0},
			expValue: pair{1, 0},
		},
		{
			name:     "increment b",
			init:     pair{0, 0},
			add:      pair{0, 1},
			expValue: pair{0, 1},
		},
		{
			name:     "increment a b",
			init:     pair{0, 0},
			add:      pair{1, 1},
			expValue: pair{1, 1},
		},

		// Decrease cases.
		{
			name:     "decrement a",
			init:     pair{1, 1},
			add:      pair{minusOne, 0},
			expValue: pair{0, 1},
		},
		{
			name:     "decrement b",
			init:     pair{1, 1},
			add:      pair{0, minusOne},
			expValue: pair{1, 0},
		},
		{
			name:     "decrement a b",
			init:     pair{1, 1},
			add:      pair{minusOne, minusOne},
			expValue: pair{0, 0},
		},

		// Increase: overflow cases.
		{
			name:     "increment overflow a",
			init:     pair{maxU32, maxU32},
			add:      pair{1, 0},
			expValue: pair{0, maxU32},
			expGen:   pair{1, 0},
		},
		{
			name:     "increment overflow b",
			init:     pair{maxU32, maxU32},
			add:      pair{0, 1},
			expValue: pair{maxU32, 0},
			expGen:   pair{0, 1},
		},
		{
			name:     "increment overflow a b",
			init:     pair{maxU32, maxU32},
			add:      pair{1, 1},
			expValue: pair{0, 0},
			expGen:   pair{1, 1},
		},
		// Decrease: overflow cases.
		{
			name:     "decrement overflow a",
			init:     pair{0, 0},
			add:      pair{minusOne, 0},
			expValue: pair{maxU32, 0},
			expGen:   pair{3, 0},
		},
		{
			name:     "decrement overflow b",
			init:     pair{0, 0},
			add:      pair{0, minusOne},
			expValue: pair{0, maxU32},
			expGen:   pair{0, 3},
		},
		{
			name:     "decrement overflow a b",
			init:     pair{0, 0},
			add:      pair{minusOne, minusOne},
			expValue: pair{maxU32, maxU32},
			expGen:   pair{3, 3},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var d dcounter
			d.Store(
				test.init.a,
				test.init.b,
			)
			a, b := d.Add(test.add.a, test.add.b)
			if act, exp := value(a), test.expValue.a; act != exp {
				t.Errorf("unexpected left value: %v; want %v", act, exp)
			}
			if act, exp := value(b), test.expValue.b; act != exp {
				t.Errorf("unexpected right value: %v; want %v", act, exp)
			}
			if act, exp := generation(a), test.expGen.a; act != exp {
				t.Errorf("unexpected left generation: %v; want %v", act, exp)
			}
			if act, exp := generation(b), test.expGen.b; act != exp {
				t.Errorf("unexpected right generation: %v; want %v", act, exp)
			}
		})
	}
}

func TestCompareTickets(t *testing.T) {
	for _, test := range []struct {
		name string
		t1   uint32
		t2   uint32
		exp  int
		err  bool
	}{
		{
			t1:  makebits(0, 0),
			t2:  makebits(2, 0),
			err: true,
		},
		{
			t1:  makebits(1, 0),
			t2:  makebits(3, 0),
			err: true,
		},
		{
			t1:  makebits(2, 0),
			t2:  makebits(0, 0),
			err: true,
		},
		{
			t1:  makebits(3, 0),
			t2:  makebits(1, 0),
			err: true,
		},

		{
			t1:  makebits(3, 0),
			t2:  makebits(0, 0),
			exp: -1,
		},
		{
			t1:  makebits(0, 0),
			t2:  makebits(3, 0),
			exp: 1,
		},

		{
			t1:  makebits(1, 0),
			t2:  makebits(0, 0),
			exp: 1,
		},
		{
			t1:  makebits(0, 0),
			t2:  makebits(1, 0),
			exp: -1,
		},

		{
			t1:  makebits(0, 1),
			t2:  makebits(0, 0),
			exp: 1,
		},
		{
			t1:  makebits(0, 0),
			t2:  makebits(0, 1),
			exp: -1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				err := recover()
				if test.err && err == nil {
					t.Errorf("no panic")
				}
				if !test.err && err != nil {
					t.Errorf("unexpected panic: %v", err)
				}
			}()
			t1 := test.t1
			t2 := test.t2
			act := comparebits(t1, t2)
			if exp := test.exp; act != exp {
				t.Errorf(
					"unexpected comparebits(%#x, %#x) result: %v; want %v",
					t1, t2, act, exp,
				)
			}
		})
	}
}
