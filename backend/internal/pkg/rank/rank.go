// Package rank implements LexoRank-style string ordering keys (ADR-009).
//
// A rank is a non-empty string over a fixed base-36 alphabet whose byte order
// equals lexicographic order, so PostgreSQL `ORDER BY rank` and Go's `<` agree.
// Each key is read as a base-36 fraction in the open interval (0,1): "i" is
// ~0.5, "9" ~0.25, "z" ~0.97. Because the reals are dense, a new key can always
// be minted strictly between two neighbours in O(1) — no row reshuffle on a
// kanban move (contrast integer positions, O(n)).
//
// Repeated inserts between the same pair grow the key length; when it crosses
// RebalanceThreshold the column should be rebalanced (Rebalance) on a background
// pass. Callers persist the string verbatim in tasks.rank / columns as needed.
package rank

import (
	"errors"
	"strings"
)

// alphabet is base-36 and strictly ascending in ASCII, so string comparison is
// the same as index comparison. Index 0 ('0') is the low digit; index 35 ('z')
// the high digit.
const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

const base = len(alphabet) // 36

// mid is the middle digit, used as the canonical "halfway" key.
const mid = "i" // alphabet[18]

// RebalanceThreshold is the key length past which a column should be rebalanced
// (ADR-009: rebalance job when key length > 64).
const RebalanceThreshold = 64

// ErrOutOfOrder is returned by Between when a >= b (no key can exist between).
var ErrOutOfOrder = errors.New("rank: lower bound not strictly below upper bound")

// idx returns the alphabet position of an ASCII digit byte.
func idx(c byte) int { return strings.IndexByte(alphabet, c) }

// Between returns a key strictly between a and b. An empty a means "before all"
// (negative infinity); an empty b means "after all" (positive infinity). With
// both empty it returns the canonical middle key. Returns ErrOutOfOrder when
// both bounds are present and a is not strictly below b.
func Between(a, b string) (string, error) {
	switch {
	case a == "" && b == "":
		return mid, nil
	case a == "":
		return before(b), nil
	case b == "":
		return after(a), nil
	default:
		if a >= b {
			return "", ErrOutOfOrder
		}
		return between(a, b), nil
	}
}

// Append returns a key ordering after last (the current maximum). Passing an
// empty last yields the canonical first key.
func Append(last string) string {
	if last == "" {
		return mid
	}
	return after(last)
}

// Prepend returns a key ordering before first (the current minimum). Passing an
// empty first yields the canonical first key.
func Prepend(first string) string {
	if first == "" {
		return mid
	}
	return before(first)
}

// NeedsRebalance reports whether a key has grown long enough to warrant a
// column rebalance (ADR-009).
func NeedsRebalance(r string) bool { return len(r) > RebalanceThreshold }

// Initial returns n evenly spaced keys for seeding or rebalancing a column,
// in ascending order. n <= 0 yields nil.
func Initial(n int) []string {
	if n <= 0 {
		return nil
	}
	// Choose a width L so base^L strictly exceeds n+1, giving each slot a
	// distinct integer coordinate; key k sits at floor((k+1)*base^L/(n+1)).
	span := 1
	width := 1
	for span <= n+1 {
		span *= base
		width++
	}
	out := make([]string, n)
	for k := 0; k < n; k++ {
		coord := (k + 1) * span / (n + 1)
		out[k] = encode(coord, width)
	}
	return out
}

// Rebalance redistributes an ordered column into fresh, short, evenly spaced
// keys. It preserves the order of the input; callers map old→new positionally.
func Rebalance(ordered []string) []string { return Initial(len(ordered)) }

// encode renders a non-negative integer as a width-digit base-36 string,
// most-significant digit first.
func encode(v, width int) string {
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = alphabet[v%base]
		v /= base
	}
	return string(buf)
}

// between returns a key strictly between a and b, both non-empty with a < b.
func between(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	prefix := a[:i]
	if i == len(a) {
		// a is a proper prefix of b: descend below b's remaining tail.
		return prefix + before(b[i:])
	}
	da, db := idx(a[i]), idx(b[i])
	if db-da >= 2 {
		return prefix + string(alphabet[(da+db)/2])
	}
	// Adjacent digits: keep a's digit and rise above its tail.
	return prefix + string(a[i]) + after(a[i+1:])
}

// before returns a key strictly below s (and above negative infinity). s is a
// valid non-empty key.
func before(s string) string {
	d := idx(s[0])
	if d >= 2 {
		return string(alphabet[d/2]) // single digit in (0, d)
	}
	// Leading digit is 0 or 1: c must share the low path, then take the middle.
	return "0" + beforeTail(s[1:])
}

// beforeTail returns a key strictly below the fraction 0.s (s possibly empty,
// meaning 0), used once a low leading "0" has been fixed.
func beforeTail(s string) string {
	if s == "" {
		return mid
	}
	return before(s)
}

// after returns a key strictly above lo (and below positive infinity). An empty
// lo (an exhausted slot tail) resolves to the middle digit.
func after(lo string) string {
	if lo == "" {
		return mid
	}
	last := len(lo) - 1
	if d := idx(lo[last]); d < base-1 {
		return lo[:last] + string(alphabet[d+1])
	}
	return lo + mid
}
