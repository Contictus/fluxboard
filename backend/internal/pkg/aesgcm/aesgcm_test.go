package aesgcm

import (
	"strings"
	"testing"
)

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRoundTrip(t *testing.T) {
	c := newTestCipher(t)
	plain := "JBSWY3DPEHPK3PXP" // a base32 TOTP secret
	ct, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if ct == plain {
		t.Fatal("ciphertext must differ from plaintext")
	}
	got, err := c.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("round trip mismatch: %q", got)
	}
}

func TestNonceIsRandom(t *testing.T) {
	c := newTestCipher(t)
	a, _ := c.Encrypt("x")
	b, _ := c.Encrypt("x")
	if a == b {
		t.Fatal("same plaintext must encrypt to different ciphertext (random nonce)")
	}
}

func TestTamperFails(t *testing.T) {
	c := newTestCipher(t)
	ct, _ := c.Encrypt("secret")
	// Flip a character in the middle of the base64 body.
	tampered := ct[:len(ct)-2] + strings.Map(func(r rune) rune {
		if r == 'A' {
			return 'B'
		}
		return 'A'
	}, ct[len(ct)-2:])
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext must fail authentication")
	}
}

func TestBadKeySize(t *testing.T) {
	if _, err := New([]byte("short")); err == nil {
		t.Fatal("expected error for wrong key size")
	}
}

func TestParseKey(t *testing.T) {
	// 32 bytes base64-encoded.
	raw := make([]byte, KeySize)
	if _, err := ParseKey("not-base64!!!==="); err == nil {
		t.Fatal("expected error for invalid base64")
	}
	// Build a valid std-base64 of 32 bytes.
	valid := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" // 32 zero bytes
	k, err := ParseKey(valid)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(k) != KeySize {
		t.Fatalf("want %d bytes, got %d", KeySize, len(k))
	}
	_ = raw
}
