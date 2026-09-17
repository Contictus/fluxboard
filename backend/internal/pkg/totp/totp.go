// Package totp implements RFC 6238 TOTP (SHA-1, 30s window) with ±1 step skew tolerance. RFC 6238 time-based one-time passwords (SHA-1, 6
// digits, 30s step) — the second factor in docs/04-AUTH.md §4. Stdlib crypto
// only; no external dependency. Secrets are base32 (RFC 3548, no padding) so
// they drop straight into an otpauth:// provisioning URI / authenticator app.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Digits in a generated code.
	Digits = 6
	// Period is the TOTP time step.
	Period = 30 * time.Second
	// secretBytes is the shared-secret size (160 bits, the SHA-1 block-friendly
	// size recommended by RFC 4226 §4).
	secretBytes = 20
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a fresh base32-encoded shared secret.
func GenerateSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("totp: rand: %w", err)
	}
	return b32.EncodeToString(b), nil
}

// Code returns the TOTP for secret at time t. Mainly used by tests and to build
// a login-time comparison; callers validating user input should use Validate.
func Code(secret string, t time.Time) (string, error) {
	key, err := b32.DecodeString(normalize(secret))
	if err != nil {
		return "", fmt.Errorf("totp: bad secret: %w", err)
	}
	return hotp(key, counter(t)), nil
}

// Validate reports whether code is valid for secret at time t, accepting the
// current step and ±1 neighbour (clock skew tolerance, docs/04-AUTH.md §4).
// Comparison is constant-time.
func Validate(secret, code string, t time.Time) bool {
	key, err := b32.DecodeString(normalize(secret))
	if err != nil {
		return false
	}
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return false
	}
	c := counter(t)
	for _, delta := range []int64{0, -1, 1} {
		if subtle.ConstantTimeCompare([]byte(hotp(key, uint64(int64(c)+delta))), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// ProvisioningURI builds the otpauth:// URI an authenticator app scans.
func ProvisioningURI(secret, issuer, account string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", Digits))
	q.Set("period", fmt.Sprintf("%d", int(Period.Seconds())))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// counter is the RFC 6238 time counter: floor(unix / period).
func counter(t time.Time) uint64 {
	return uint64(t.Unix() / int64(Period.Seconds()))
}

// hotp is RFC 4226 HOTP with SHA-1 dynamic truncation.
func hotp(key []byte, ctr uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], ctr)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])
	mod := bin % 1_000_000 // 10^Digits
	return fmt.Sprintf("%0*d", Digits, mod)
}

// normalize strips spaces and uppercases so codes copied with formatting still
// decode.
func normalize(secret string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
}
