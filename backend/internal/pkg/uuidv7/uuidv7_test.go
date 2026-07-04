package uuidv7

import (
	"testing"
	"time"
)

func TestNewAtEncodesVersionAndVariant(t *testing.T) {
	u := NewAt(time.UnixMilli(0x0102030405))
	if got := u[6] >> 4; got != 0x7 {
		t.Fatalf("version = %x, want 7", got)
	}
	if got := u[8] >> 6; got != 0x2 {
		t.Fatalf("variant = %b, want 10", got)
	}
}

func TestNewIsTimeOrdered(t *testing.T) {
	a := NewAt(time.UnixMilli(1000))
	b := NewAt(time.UnixMilli(2000))
	if a.String()[:12] >= b.String()[:12] {
		t.Fatalf("earlier UUID %s not < later %s", a, b)
	}
}

func TestStringFormat(t *testing.T) {
	s := New().String()
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		t.Fatalf("bad canonical form: %q", s)
	}
}
