package authuc

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/token"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/totp"
)

// totpIssuer is the label shown in the authenticator app.
const totpIssuer = "Fluxboard"

// recoveryCodeCount is how many single-use recovery codes are issued on activate.
const recoveryCodeCount = 10

// EnrollResult carries the freshly generated (not-yet-active) TOTP secret and a
// provisioning URI for the QR code. The secret is shown once; the account is not
// protected until Activate2FA confirms a code.
type EnrollResult struct {
	Secret          string
	ProvisioningURI string
}

// Enroll2FA generates a TOTP secret, stores it encrypted with enabled=false, and
// returns it for the user to add to their authenticator (docs/04-AUTH.md §5).
func (s *Service) Enroll2FA(ctx context.Context, userID string) (EnrollResult, error) {
	if s.cipher == nil {
		return EnrollResult{}, fmt.Errorf("2fa not configured: %w", domain.ErrForbidden)
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return EnrollResult{}, err
	}
	if u.TOTPEnabled {
		return EnrollResult{}, domain.ErrConflict // already enabled; disable first
	}
	secret, err := totp.GenerateSecret()
	if err != nil {
		return EnrollResult{}, err
	}
	enc, err := s.cipher.Encrypt(secret)
	if err != nil {
		return EnrollResult{}, err
	}
	if err := s.users.SetTOTP(ctx, u.ID, enc, false); err != nil {
		return EnrollResult{}, err
	}
	return EnrollResult{
		Secret:          secret,
		ProvisioningURI: totp.ProvisioningURI(secret, totpIssuer, u.Email),
	}, nil
}

// Activate2FA verifies a code against the pending secret, flips enabled=true, and
// issues a fresh set of recovery codes (returned once, plaintext).
func (s *Service) Activate2FA(ctx context.Context, userID, code string) ([]string, error) {
	if s.cipher == nil {
		return nil, fmt.Errorf("2fa not configured: %w", domain.ErrForbidden)
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.TOTPEnabled {
		return nil, domain.ErrConflict
	}
	if u.TOTPSecret == "" {
		return nil, fmt.Errorf("not enrolled: %w", domain.ErrValidation)
	}
	secret, err := s.cipher.Decrypt(u.TOTPSecret)
	if err != nil {
		return nil, err
	}
	if !totp.Validate(secret, code, s.clock.Now()) {
		return nil, fmt.Errorf("invalid code: %w", domain.ErrValidation)
	}
	// Enable, keeping the same encrypted secret.
	if err := s.users.SetTOTP(ctx, u.ID, u.TOTPSecret, true); err != nil {
		return nil, err
	}
	codes, hashes, err := genRecoveryCodes(recoveryCodeCount)
	if err != nil {
		return nil, err
	}
	if err := s.recovery.Replace(ctx, u.ID, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

// Disable2FA turns off TOTP after re-verifying a current code (so a hijacked but
// unprivileged session cannot strip the second factor), clearing the secret and
// all recovery codes.
func (s *Service) Disable2FA(ctx context.Context, userID, code string) error {
	if s.cipher == nil {
		return fmt.Errorf("2fa not configured: %w", domain.ErrForbidden)
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !u.TOTPEnabled {
		return nil // idempotent: already off
	}
	if !s.verifyTOTPOrRecovery(ctx, u.ID, u.TOTPSecret, code) {
		return domain.ErrUnauthorized
	}
	if err := s.users.SetTOTP(ctx, u.ID, "", false); err != nil {
		return err
	}
	return s.recovery.Replace(ctx, u.ID, nil) // drop all recovery codes
}

// Verify2FA exchanges a pending-2FA token + TOTP/recovery code for a session
// (docs/04-AUTH.md §4).
func (s *Service) Verify2FA(ctx context.Context, pendingToken, code, userAgent, ip string) (Tokens, error) {
	if s.verifier == nil || s.cipher == nil {
		return Tokens{}, fmt.Errorf("2fa not configured: %w", domain.ErrForbidden)
	}
	userID, err := s.verifier.VerifyPending2FA(pendingToken)
	if err != nil {
		return Tokens{}, domain.ErrUnauthorized
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Tokens{}, domain.ErrUnauthorized
		}
		return Tokens{}, err
	}
	if !u.TOTPEnabled {
		return Tokens{}, domain.ErrUnauthorized // token stale: 2FA turned off
	}
	if !s.verifyTOTPOrRecovery(ctx, u.ID, u.TOTPSecret, code) {
		s.observe(ctx).LoginAttempt(ctx, "invalid")
		return Tokens{}, domain.ErrUnauthorized
	}
	tokens, err := s.issueSession(ctx, u.ID, userAgent, ip)
	if err != nil {
		return Tokens{}, err
	}
	s.observe(ctx).LoginAttempt(ctx, "success")
	return tokens, nil
}

// verifyTOTPOrRecovery accepts either a valid TOTP for the (encrypted) secret or
// an unused recovery code (which it consumes). Errors decrypting the secret fail
// closed.
func (s *Service) verifyTOTPOrRecovery(ctx context.Context, userID, encSecret, code string) bool {
	secret, err := s.cipher.Decrypt(encSecret)
	if err == nil && totp.Validate(secret, code, s.clock.Now()) {
		return true
	}
	// Recovery-code fallback: codes are stored as upper-case base32; normalize
	// the input so formatting/case differences still match.
	normalized := strings.ToUpper(strings.TrimSpace(code))
	ok, err := s.recovery.Consume(ctx, userID, token.Hash(normalized))
	return err == nil && ok
}

// genRecoveryCodes returns n human-typable recovery codes and their storage
// hashes. Each code is 10 base32 chars (~50 bits) — unguessable, single-use.
func genRecoveryCodes(n int) (codes []string, hashes [][]byte, err error) {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	codes = make([]string, 0, n)
	hashes = make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		b := make([]byte, 7) // 7 bytes -> 11 base32 chars; trim to 10
		if _, err := rand.Read(b); err != nil {
			return nil, nil, fmt.Errorf("recovery codes: %w", err)
		}
		c := enc.EncodeToString(b)[:10]
		codes = append(codes, c)
		hashes = append(hashes, token.Hash(c))
	}
	return codes, hashes, nil
}
