package jwtx

import (
	"testing"
	"time"
)

func newTestSigner(t *testing.T) *Signer {
	t.Helper()
	pemStr, err := GenerateES256PEM()
	if err != nil {
		t.Fatal(err)
	}
	priv, err := LoadPrivateKeyPEM(pemStr)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSigner(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSignVerifyRoundTrip(t *testing.T) {
	s := newTestSigner(t)
	v := VerifierFromSigners(s)

	tok, err := s.Sign("user-123", "sess-abc", time.Now(), 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := v.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "user-123" || claims.SID != "sess-abc" {
		t.Fatalf("claims mismatch: sub=%q sid=%q", claims.Subject, claims.SID)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	s := newTestSigner(t)
	v := VerifierFromSigners(s)
	tok, _ := s.Sign("u", "sid", time.Now().Add(-time.Hour), 15*time.Minute)
	if _, err := v.Verify(tok); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestVerifyRejectsUnknownKID(t *testing.T) {
	signer := newTestSigner(t)
	other := newTestSigner(t)
	// Verifier only knows `other`; token signed by `signer` -> unknown kid.
	v := VerifierFromSigners(other)
	tok, _ := signer.Sign("u", "sid", time.Now(), 15*time.Minute)
	if _, err := v.Verify(tok); err == nil {
		t.Fatal("expected unknown-kid token to be rejected")
	}
}
