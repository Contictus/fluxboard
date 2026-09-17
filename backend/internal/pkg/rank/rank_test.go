package rank

import (
	"math/rand"
	"sort"
	"testing"
)

// asc reports whether every adjacent pair is strictly ascending.
func asc(t *testing.T, keys []string) {
	t.Helper()
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("not strictly ascending at %d: %q >= %q", i, keys[i-1], keys[i])
		}
	}
}

func TestBetweenBasic(t *testing.T) {
	cases := []struct{ a, b string }{
		{"1", "3"},
		{"1", "2"},
		{"15", "16"},
		{"1", "15"},
		{"a", "b"},
		{"11", "12"},
		{"110", "111"},
	}
	for _, c := range cases {
		got, err := Between(c.a, c.b)
		if err != nil {
			t.Fatalf("Between(%q,%q) err: %v", c.a, c.b, err)
		}
		if c.a >= got || got >= c.b {
			t.Fatalf("Between(%q,%q)=%q not strictly between", c.a, c.b, got)
		}
	}
}

func TestOpenBounds(t *testing.T) {
	first := Append("") // canonical first
	if first == "" {
		t.Fatal("Append(\"\") empty")
	}
	// Append past a value is greater; Prepend before it is lesser.
	hi := Append(first)
	if hi <= first {
		t.Fatalf("Append(%q)=%q not greater", first, hi)
	}
	lo := Prepend(first)
	if lo >= first {
		t.Fatalf("Prepend(%q)=%q not lesser", first, lo)
	}
	// Between with sentinels.
	if _, err := Between(first, ""); err != nil {
		t.Fatalf("Between(first,\"\") err: %v", err)
	}
	if _, err := Between("", first); err != nil {
		t.Fatalf("Between(\"\",first) err: %v", err)
	}
}

func TestOutOfOrder(t *testing.T) {
	if _, err := Between("b", "a"); err != ErrOutOfOrder {
		t.Fatalf("want ErrOutOfOrder, got %v", err)
	}
	if _, err := Between("a", "a"); err != ErrOutOfOrder {
		t.Fatalf("equal bounds want ErrOutOfOrder, got %v", err)
	}
}

func TestPrependChainStaysAboveFloor(t *testing.T) {
	// Repeatedly prepend; every key must stay strictly ordered and non-empty.
	keys := []string{Append("")}
	for i := 0; i < 500; i++ {
		k := Prepend(keys[0])
		if k == "" || k >= keys[0] {
			t.Fatalf("prepend %d produced %q (head %q)", i, k, keys[0])
		}
		keys = append([]string{k}, keys...)
	}
	asc(t, keys)
}

func TestInitialEvenlySpaced(t *testing.T) {
	for _, n := range []int{1, 2, 5, 36, 100} {
		keys := Initial(n)
		if len(keys) != n {
			t.Fatalf("Initial(%d) len=%d", n, len(keys))
		}
		asc(t, keys)
	}
}

// TestRandomMovesTotalOrder is the docs/12 §3 property test: 10k random moves
// (insert a key between two random neighbours) must preserve total order.
func TestRandomMovesTotalOrder(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	// Seed a small ordered list.
	list := Initial(5)
	for move := 0; move < 10000; move++ {
		// Pick an insertion gap [i-1, i] in the current ordered list.
		i := rng.Intn(len(list) + 1)
		var a, b string
		if i > 0 {
			a = list[i-1]
		}
		if i < len(list) {
			b = list[i]
		}
		k, err := Between(a, b)
		if err != nil {
			t.Fatalf("move %d Between(%q,%q): %v", move, a, b, err)
		}
		if a != "" && a >= k {
			t.Fatalf("move %d: %q !< %q", move, a, k)
		}
		if b != "" && k >= b {
			t.Fatalf("move %d: %q !< %q", move, k, b)
		}
		// Insert keeping order.
		list = append(list, "")
		copy(list[i+1:], list[i:])
		list[i] = k
	}
	// Final list must be strictly ascending and match a fresh sort.
	asc(t, list)
	sorted := append([]string(nil), list...)
	sort.Strings(sorted)
	for i := range list {
		if list[i] != sorted[i] {
			t.Fatalf("order diverged at %d", i)
		}
	}
}

func TestNeedsRebalance(t *testing.T) {
	short := Append("")
	if NeedsRebalance(short) {
		t.Fatalf("%q flagged for rebalance", short)
	}
	// Force growth: repeatedly insert just above a fixed floor. Each midpoint
	// sits between "a" and the previous key, lengthening the string.
	k := "z"
	for i := 0; i < 2000 && !NeedsRebalance(k); i++ {
		var err error
		k, err = Between("a", k)
		if err != nil {
			t.Fatalf("grow err: %v", err)
		}
	}
	if !NeedsRebalance(k) {
		t.Fatalf("expected long key to need rebalance, len=%d", len(k))
	}
	// Rebalance yields short, ordered keys.
	rb := Rebalance(make([]string, 10))
	asc(t, rb)
	for _, r := range rb {
		if NeedsRebalance(r) {
			t.Fatalf("rebalanced key still long: %q", r)
		}
	}
}
