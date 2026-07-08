// Package apikey is the domain for org-scoped API keys — a second authentication
// path alongside user sessions (docs/build/PHASE-6-ADMIN-OBS.md §2, FR-API-001/002).
// A key authenticates as its org (no user), carries a scope set (read|write), and
// is rate-limited by the org's plan tier. The secret is shown once at creation;
// only its SHA-256 hash is persisted. Domain is stdlib-only.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Prefix is the fixed, public head of every key secret. It marks a token as a
// Fluxboard live API key on sight (log scrubbing, secret scanners) and is stored
// alongside a short public id so the UI can identify a key without the secret.
const Prefix = "fbk_live_"

// Scope gates what an API key may do. write implies read (HasScope).
type Scope string

const (
	ScopeRead  Scope = "read"
	ScopeWrite Scope = "write"
)

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool { return s == ScopeRead || s == ScopeWrite }

// APIKey is a stored key. KeyHash is the SHA-256 (hex) of the full secret; the
// plaintext is never persisted. RevokedAt non-nil ⇒ inactive. Prefix is the
// public identifier ("fbk_live_ab12cd34").
type APIKey struct {
	ID         string
	OrgID      string
	Prefix     string
	KeyHash    string
	Name       string
	Scopes     []Scope
	CreatedBy  string
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// Active reports whether the key may authenticate at time t (not revoked).
func (k APIKey) Active() bool { return k.RevokedAt == nil }

// HasScope reports whether the key's scopes grant want. A write key also reads.
func (k APIKey) HasScope(want Scope) bool {
	for _, s := range k.Scopes {
		if s == want || (want == ScopeRead && s == ScopeWrite) {
			return true
		}
	}
	return false
}

// Generated is the output of Generate: the one-time plaintext to show the user,
// plus the derived public prefix and stored hash.
type Generated struct {
	Plaintext string // returned to the caller once; never stored
	Prefix    string // public identifier, persisted
	KeyHash   string // SHA-256 hex, persisted
}

// Generate mints a new key secret: Prefix + 32 bytes of CSPRNG entropy (hex). The
// public Prefix embeds the first 8 hex chars so a listed key is recognizable. The
// plaintext is returned once; only Prefix and KeyHash are meant to be persisted.
func Generate() (Generated, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return Generated{}, fmt.Errorf("apikey: read entropy: %w", err)
	}
	secret := hex.EncodeToString(buf)
	plaintext := Prefix + secret
	return Generated{
		Plaintext: plaintext,
		Prefix:    Prefix + secret[:8],
		KeyHash:   Hash(plaintext),
	}, nil
}

// Hash returns the SHA-256 (hex) of a full key secret — the value stored and the
// lookup key for ResolveByKey.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// Verify reports whether plaintext hashes to storedHash, in constant time.
func Verify(plaintext, storedHash string) bool {
	got := Hash(plaintext)
	return subtle.ConstantTimeCompare([]byte(got), []byte(storedHash)) == 1
}

// LooksLikeKey reports whether a bearer credential is shaped like an API key (vs a
// JWT session token) — the cheap discriminator the auth branch uses before hashing.
func LooksLikeKey(token string) bool { return strings.HasPrefix(token, Prefix) }
