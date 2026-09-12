// Package argon2x wraps Argon2id password hashing and the registration/reset
// password policy (docs/04-AUTH.md §1). Parameters are pinned and guarded by a
// unit test so they cannot be silently downgraded.
//
// Leaf package: no domain imports; policy errors surface as 400s at the usecase boundary.: it returns its own policy errors and never imports
// domain; the usecase layer maps PolicyError to domain.ErrValidation.
package argon2x

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/alexedwards/argon2id"
)

// Params are the pinned Argon2id parameters (OWASP-aligned). Changing these is a
// security-relevant decision; TestParams guards against accidental drift.
var Params = &argon2id.Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

const (
	minPasswordLen = 12
	maxPasswordLen = 128
)

// PolicyError describes why a password was rejected. The usecase maps it to a
// validation error; the message is safe to show the user.
type PolicyError struct{ Reason string }

func (e PolicyError) Error() string { return e.Reason }

//go:embed breaches.txt
var breachData string

// breachSet is the lowercase breached-password set, loaded once. In production
// this is the top-10k list; the committed file is a representative subset — the
// mechanism (embed.FS, lowercase exact match) is what matters here.
var breachSet = loadBreaches(breachData)

func loadBreaches(data string) map[string]struct{} {
	m := make(map[string]struct{})
	for _, line := range strings.Split(data, "\n") {
		if p := strings.TrimSpace(strings.ToLower(line)); p != "" {
			m[p] = struct{}{}
		}
	}
	return m
}

// Hash returns the encoded Argon2id hash (PHC string) for the password.
func Hash(password string) (string, error) {
	h, err := argon2id.CreateHash(password, Params)
	if err != nil {
		return "", fmt.Errorf("argon2x hash: %w", err)
	}
	return h, nil
}

// Verify reports whether password matches the encoded hash (constant-time).
func Verify(password, encodedHash string) (bool, error) {
	ok, err := argon2id.ComparePasswordAndHash(password, encodedHash)
	if err != nil {
		return false, fmt.Errorf("argon2x verify: %w", err)
	}
	return ok, nil
}

// DummyHash is a valid encoded hash of a random password, generated at startup.
// Login verifies against it for unknown emails so response timing does not
// reveal whether an account exists (docs/04-AUTH.md §4).
var DummyHash = mustDummyHash()

func mustDummyHash() string {
	h, err := Hash("dummy-password-for-uniform-login-timing")
	if err != nil {
		panic("argon2x: cannot build dummy hash: " + err.Error())
	}
	return h
}

// CheckPolicy validates a candidate password against the registration/reset
// policy (docs/04-AUTH.md §1): length bounds, not breached, not the email local
// part. Returns a PolicyError on the first failure.
func CheckPolicy(password, email string) error {
	if n := len(password); n < minPasswordLen || n > maxPasswordLen {
		return PolicyError{Reason: fmt.Sprintf("password must be %d–%d characters", minPasswordLen, maxPasswordLen)}
	}
	if _, bad := breachSet[strings.ToLower(password)]; bad {
		return PolicyError{Reason: "password appears in a known breach list"}
	}
	local := email
	if at := strings.IndexByte(email, '@'); at != -1 {
		local = email[:at]
	}
	if local != "" && strings.EqualFold(password, local) {
		return PolicyError{Reason: "password must not match your email"}
	}
	return nil
}

// IsPolicyError reports whether err is a password-policy rejection.
func IsPolicyError(err error) bool {
	var pe PolicyError
	return errors.As(err, &pe)
}
