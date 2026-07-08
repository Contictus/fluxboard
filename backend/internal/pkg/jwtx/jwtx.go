// Package jwtx signs and verifies ES256 access JWTs (docs/04-AUTH.md §2). The
// verifier holds a small kid-keyed JWKS (current + previous key) so key
// rotation is "publish new, keep old until the max 15-min access TTL passes".
package jwtx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	Issuer   = "fluxboard"
	Audience = "fluxboard-api"
)

// ScopePending2FA marks a pending-2FA JWT (docs/04-AUTH.md §4): issued after a
// correct password when TOTP is enabled, exchanged at /auth/2fa/verify for a
// real session. It is NOT an access token.
const ScopePending2FA = "pending_2fa"

// Claims is the access-token payload (docs/04-AUTH.md §2). No role/tenant claims
// live here (ADR 008): authorization is resolved per request, never baked into
// the token, so a revoked privilege applies immediately. Scope is empty for
// access tokens and set for special-purpose tokens (e.g. pending_2fa).
type Claims struct {
	SID   string `json:"sid,omitempty"`
	Scope string `json:"scope,omitempty"`
	Ver   bool   `json:"ver,omitempty"` // email verified at issue time (FR-AUTH-002 gate)
	Imp   string `json:"imp,omitempty"` // impersonated org id (platform-admin read-only, FR-ADM-003)
	jwt.RegisteredClaims
}

// Signer issues tokens with one private key; its kid is derived from the public
// key so it is stable and collision-resistant.
type Signer struct {
	kid string
	key *ecdsa.PrivateKey
}

// NewSigner builds a Signer from an ECDSA (P-256) private key.
func NewSigner(priv *ecdsa.PrivateKey) (*Signer, error) {
	if priv.Curve != elliptic.P256() {
		return nil, errors.New("jwtx: ES256 requires a P-256 key")
	}
	kid, err := publicKID(&priv.PublicKey)
	if err != nil {
		return nil, err
	}
	return &Signer{kid: kid, key: priv}, nil
}

// KID returns the key id stamped into the JWT header.
func (s *Signer) KID() string { return s.kid }

// PublicKey returns the verification key (feed it into NewVerifier).
func (s *Signer) PublicKey() *ecdsa.PublicKey { return &s.key.PublicKey }

// Sign issues a signed access token for subject/sid valid for ttl from now. The
// verified flag records whether the user's email was verified at issue time so
// the RequireVerified gate need not hit the DB per request (FR-AUTH-002).
func (s *Signer) Sign(subject, sid string, verified bool, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		SID: sid,
		Ver: verified,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = s.kid
	signed, err := tok.SignedString(s.key)
	if err != nil {
		return "", fmt.Errorf("jwtx sign: %w", err)
	}
	return signed, nil
}

// SignPending2FA issues a short-lived JWT (scope=pending_2fa, no sid) that the
// client exchanges for a session after passing the TOTP step (docs/04-AUTH.md §4).
func (s *Signer) SignPending2FA(subject string, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		Scope: ScopePending2FA,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = s.kid
	signed, err := tok.SignedString(s.key)
	if err != nil {
		return "", fmt.Errorf("jwtx sign pending: %w", err)
	}
	return signed, nil
}

// SignImpersonation issues a read-only impersonation access token (docs/11 §admin,
// FR-ADM-003): subject = the platform admin, sid = the admin's live session (so
// session revocation still applies), imp = the target org id. It is a normal access
// token (empty scope) plus the imp claim; the tenant guard grants read-only access
// to imp's org and a write-guard rejects any mutation carrying it.
func (s *Signer) SignImpersonation(adminUserID, adminSID, targetOrg string, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		SID: adminSID,
		Ver: true,
		Imp: targetOrg,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   adminUserID,
			Audience:  jwt.ClaimStrings{Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = s.kid
	signed, err := tok.SignedString(s.key)
	if err != nil {
		return "", fmt.Errorf("jwtx sign impersonation: %w", err)
	}
	return signed, nil
}

// Verifier validates tokens against a kid->public-key set (1–2 keys).
type Verifier struct {
	keys map[string]*ecdsa.PublicKey
}

// NewVerifier builds a verifier from a kid-keyed public-key set. Use
// VerifierFromSigners to derive it from the active signer(s).
func NewVerifier(keys map[string]*ecdsa.PublicKey) *Verifier {
	return &Verifier{keys: keys}
}

// VerifierFromSigners assembles a JWKS from one or more signers (current first,
// previous next) for rotation.
func VerifierFromSigners(signers ...*Signer) *Verifier {
	keys := make(map[string]*ecdsa.PublicKey, len(signers))
	for _, s := range signers {
		keys[s.kid] = s.PublicKey()
	}
	return &Verifier{keys: keys}
}

// Verify checks signature, alg, issuer, audience, and expiry, returning the
// claims on success. Alg is restricted to ES256 (no alg-confusion).
func (v *Verifier) Verify(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		pub, ok := v.keys[kid]
		if !ok {
			return nil, fmt.Errorf("jwtx: unknown kid %q", kid)
		}
		return pub, nil
	},
		jwt.WithValidMethods([]string{"ES256"}),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(Audience),
	)
	if err != nil {
		return nil, fmt.Errorf("jwtx verify: %w", err)
	}
	// An access token must carry no scope: a pending-2FA (or any special-purpose)
	// token must never be accepted as a bearer credential.
	if claims.Scope != "" {
		return nil, fmt.Errorf("jwtx: unexpected token scope %q", claims.Scope)
	}
	return claims, nil
}

// VerifyPending2FA validates a pending-2FA token and returns its subject
// (user id). It enforces scope=pending_2fa so an access token cannot be replayed
// here and vice-versa (docs/04-AUTH.md §4).
func (v *Verifier) VerifyPending2FA(tokenStr string) (userID string, err error) {
	claims := &Claims{}
	_, err = jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		pub, ok := v.keys[kid]
		if !ok {
			return nil, fmt.Errorf("jwtx: unknown kid %q", kid)
		}
		return pub, nil
	},
		jwt.WithValidMethods([]string{"ES256"}),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(Audience),
	)
	if err != nil {
		return "", fmt.Errorf("jwtx verify pending: %w", err)
	}
	if claims.Scope != ScopePending2FA {
		return "", fmt.Errorf("jwtx: not a pending_2fa token")
	}
	return claims.Subject, nil
}

// LoadPrivateKeyPEM parses an ECDSA private key from a PKCS#8 or SEC1 PEM.
func LoadPrivateKeyPEM(pemStr string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("jwtx: no PEM block found")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		ec, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("jwtx: PKCS#8 key is not ECDSA")
		}
		return ec, nil
	}
	ec, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("jwtx: parse EC private key: %w", err)
	}
	return ec, nil
}

// GenerateES256PEM creates a new P-256 private key and returns it PKCS#8-PEM
// encoded. Used to mint a dev key (see Makefile gen-jwt-key) and in tests.
func GenerateES256PEM() (string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

// publicKID derives a short, stable kid from the public key's DER SHA-256.
func publicKID(pub *ecdsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:8]), nil
}
