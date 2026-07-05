package totp

import (
	"testing"
	"time"
)

// TestRFC6238Vector checks a known RFC 6238 test vector (SHA-1, secret
// "12345678901234567890" = base32 GEZD...). At T=59s the 8-digit code is
// 94287082; our 6-digit truncation is its last six digits.
func TestRFC6238Vector(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // base32("12345678901234567890")
	got, err := Code(secret, time.Unix(59, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got != "287082" {
		t.Fatalf("want 287082, got %s", got)
	}
}

func TestValidateAcceptsCurrentAndNeighbours(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	code, _ := Code(secret, now)
	if !Validate(secret, code, now) {
		t.Fatal("current code should validate")
	}
	// Code from the previous step must still pass within the same step's grace.
	prev, _ := Code(secret, now.Add(-Period))
	if !Validate(secret, prev, now) {
		t.Fatal("previous-step code should validate (±1 window)")
	}
	// Two steps away must fail.
	far, _ := Code(secret, now.Add(-3*Period))
	if Validate(secret, far, now) {
		t.Fatal("far code should not validate")
	}
}

func TestValidateRejectsGarbage(t *testing.T) {
	secret, _ := GenerateSecret()
	if Validate(secret, "abc", time.Now()) {
		t.Fatal("non-numeric wrong-length code must fail")
	}
	if Validate(secret, "000000", time.Now().Add(1000*time.Hour)) {
		t.Fatal("unrelated code must fail")
	}
}

func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI("ABCDEF", "Fluxboard", "a@b.com")
	if uri == "" || uri[:10] != "otpauth://" {
		t.Fatalf("unexpected uri: %s", uri)
	}
}
