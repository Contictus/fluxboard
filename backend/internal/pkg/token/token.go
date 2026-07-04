// Package token generates cryptographically-random opaque tokens and their
// storage hashes. Refresh tokens, email-verify/reset links, invitations, and
// API keys are all "random secret, presented to the client; only its SHA-256
// stored" (docs/04-AUTH.md §2). Constant length + base64url = URL/cookie-safe.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// Bytes is the entropy size for every opaque token (256 bits).
const Bytes = 32

// New returns a fresh base64url (unpadded) token carrying 32 random bytes.
func New() (string, error) {
	b := make([]byte, Bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token: rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewWithPrefix returns prefix+token, e.g. token.NewWithPrefix("fbk_live_").
func NewWithPrefix(prefix string) (string, error) {
	t, err := New()
	if err != nil {
		return "", err
	}
	return prefix + t, nil
}

// Hash returns the SHA-256 of the token — the only form stored server-side.
func Hash(t string) []byte {
	sum := sha256.Sum256([]byte(t))
	return sum[:]
}

// Equal compares two hashes in constant time.
func Equal(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }
