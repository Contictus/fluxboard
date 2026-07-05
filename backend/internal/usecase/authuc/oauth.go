package authuc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// oauthStateTTL bounds how long a started OAuth flow may take to call back
// (docs/04-AUTH.md §4: 10 min).
const oauthStateTTL = 10 * time.Minute

const providerGoogle = "google"

// StartOAuth generates PKCE + state, stores the verifier, and returns the Google
// consent URL to redirect the browser to (docs/04-AUTH.md §4).
func (s *Service) StartOAuth(ctx context.Context, states auth.OAuthStateStore, redirectAfter string) (string, error) {
	if s.google == nil {
		return "", fmt.Errorf("oauth not configured: %w", domain.ErrForbidden)
	}
	state, err := randB64(32)
	if err != nil {
		return "", err
	}
	verifier, err := randB64(48) // 64 base64url chars, within 43–128 (RFC 7636)
	if err != nil {
		return "", err
	}
	if err := states.Save(ctx, state, auth.OAuthState{Verifier: verifier, RedirectAfter: redirectAfter}, oauthStateTTL); err != nil {
		return "", err
	}
	return s.google.AuthCodeURL(state, pkceChallenge(verifier)), nil
}

// OAuthCallbackResult is the outcome of a successful callback.
type OAuthCallbackResult struct {
	Tokens        Tokens
	RedirectAfter string
}

// CompleteOAuth validates state, exchanges the code, applies the identity
// linking rules (docs/04-AUTH.md §4), and issues a session.
func (s *Service) CompleteOAuth(ctx context.Context, states auth.OAuthStateStore, code, state, userAgent, ip string) (OAuthCallbackResult, error) {
	if s.google == nil {
		return OAuthCallbackResult{}, fmt.Errorf("oauth not configured: %w", domain.ErrForbidden)
	}
	st, err := states.Take(ctx, state)
	if err != nil {
		return OAuthCallbackResult{}, domain.ErrUnauthorized // unknown/expired/replayed state
	}
	profile, err := s.google.Exchange(ctx, code, st.Verifier)
	if err != nil {
		return OAuthCallbackResult{}, domain.ErrUnauthorized
	}
	if !profile.EmailVerified {
		return OAuthCallbackResult{}, fmt.Errorf("google email not verified: %w", domain.ErrForbidden)
	}

	userID, err := s.resolveOAuthUser(ctx, profile)
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	tokens, err := s.issueSession(ctx, userID, userAgent, ip)
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	s.observe(ctx).LoginAttempt(ctx, "success")
	return OAuthCallbackResult{Tokens: tokens, RedirectAfter: st.RedirectAfter}, nil
}

// resolveOAuthUser applies the four-way linking decision and returns the user id
// to log in.
func (s *Service) resolveOAuthUser(ctx context.Context, p auth.OAuthUser) (string, error) {
	// 1. Known identity → that user.
	id, err := s.oauth.GetByProviderSub(ctx, providerGoogle, p.Sub)
	if err == nil {
		return id.UserID, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return "", err
	}

	email := normalizeEmail(p.Email)
	u, err := s.users.GetByEmail(ctx, email)
	switch {
	case err == nil && u.EmailVerified:
		// 2. Existing verified account → link identity, login.
		if err := s.linkIdentity(ctx, u.ID, p.Sub); err != nil {
			return "", err
		}
		return u.ID, nil
	case err == nil && !u.EmailVerified:
		// 3. Unverified local account → refuse silent takeover.
		return "", fmt.Errorf("verify your email first: %w", domain.ErrConflict)
	case errors.Is(err, domain.ErrNotFound):
		// 4. New user: create verified, passwordless, link identity.
		return s.createOAuthUser(ctx, email, p)
	default:
		return "", err
	}
}

func (s *Service) createOAuthUser(ctx context.Context, email string, p auth.OAuthUser) (string, error) {
	now := s.clock.Now()
	u := &auth.User{
		ID:            uuidv7.New().String(),
		Email:         email,
		Name:          strings.TrimSpace(p.Name),
		EmailVerified: true,
		PlatformRole:  auth.PlatformRoleUser,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return "", err
	}
	if err := s.linkIdentity(ctx, u.ID, p.Sub); err != nil {
		return "", err
	}
	return u.ID, nil
}

func (s *Service) linkIdentity(ctx context.Context, userID, sub string) error {
	return s.oauth.Create(ctx, &auth.OAuthIdentity{
		ID:          uuidv7.New().String(),
		UserID:      userID,
		Provider:    providerGoogle,
		ProviderSub: sub,
	})
}

// pkceChallenge is the RFC 7636 S256 transform: base64url(sha256(verifier)).
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// randB64 returns n random bytes base64url-encoded (unpadded).
func randB64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
