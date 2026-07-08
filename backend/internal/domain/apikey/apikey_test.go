package apikey

import (
	"strings"
	"testing"
	"time"
)

func TestGenerate_ShapeHashPrefix(t *testing.T) {
	g, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(g.Plaintext, Prefix) {
		t.Errorf("plaintext %q missing prefix %q", g.Plaintext, Prefix)
	}
	if !LooksLikeKey(g.Plaintext) {
		t.Errorf("LooksLikeKey(%q) = false", g.Plaintext)
	}
	if !strings.HasPrefix(g.Prefix, Prefix) || len(g.Prefix) != len(Prefix)+8 {
		t.Errorf("public prefix %q malformed", g.Prefix)
	}
	if g.KeyHash != Hash(g.Plaintext) {
		t.Errorf("KeyHash mismatch")
	}
	if !Verify(g.Plaintext, g.KeyHash) {
		t.Errorf("Verify(plaintext, hash) = false")
	}
	if Verify(g.Plaintext+"x", g.KeyHash) {
		t.Errorf("Verify accepted a tampered secret")
	}
}

func TestGenerate_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		g, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		if seen[g.KeyHash] {
			t.Fatalf("duplicate hash on iteration %d", i)
		}
		seen[g.KeyHash] = true
	}
}

func TestHasScope(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		scopes []Scope
		want   Scope
		ok     bool
	}{
		{"read key reads", []Scope{ScopeRead}, ScopeRead, true},
		{"read key cannot write", []Scope{ScopeRead}, ScopeWrite, false},
		{"write key writes", []Scope{ScopeWrite}, ScopeWrite, true},
		{"write key implies read", []Scope{ScopeWrite}, ScopeRead, true},
		{"empty scopes deny", nil, ScopeRead, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := APIKey{Scopes: tc.scopes, CreatedAt: now}
			if got := k.HasScope(tc.want); got != tc.ok {
				t.Errorf("HasScope(%v) = %v, want %v", tc.want, got, tc.ok)
			}
		})
	}
}

func TestActive(t *testing.T) {
	if !(APIKey{}).Active() {
		t.Error("fresh key should be active")
	}
	now := time.Now()
	if (APIKey{RevokedAt: &now}).Active() {
		t.Error("revoked key should be inactive")
	}
}
