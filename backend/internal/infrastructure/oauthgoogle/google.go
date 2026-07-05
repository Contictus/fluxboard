// Package oauthgoogle implements auth.OAuthProvider for Google using
// golang.org/x/oauth2 with PKCE (docs/04-AUTH.md §4). It fetches the OpenID
// userinfo after the code exchange and normalizes it to auth.OAuthUser.
package oauthgoogle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
)

// userinfoEndpoint is Google's OpenID Connect userinfo URL.
const userinfoEndpoint = "https://openidconnect.googleapis.com/v1/userinfo"

// Provider is a configured Google OAuth client.
type Provider struct {
	cfg *oauth2.Config
}

// New builds a Provider from client credentials and the registered redirect URL.
func New(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{cfg: &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       []string{"openid", "email", "profile"},
	}}
}

var _ auth.OAuthProvider = (*Provider)(nil)

// AuthCodeURL builds the consent URL carrying the PKCE S256 challenge.
func (p *Provider) AuthCodeURL(state, codeChallenge string) string {
	return p.cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

// Exchange swaps code+verifier for a token and fetches the verified profile.
func (p *Provider) Exchange(ctx context.Context, code, codeVerifier string) (auth.OAuthUser, error) {
	tok, err := p.cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
	if err != nil {
		return auth.OAuthUser{}, fmt.Errorf("google exchange: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoEndpoint, nil)
	if err != nil {
		return auth.OAuthUser{}, err
	}
	resp, err := p.cfg.Client(ctx, tok).Do(req)
	if err != nil {
		return auth.OAuthUser{}, fmt.Errorf("google userinfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return auth.OAuthUser{}, fmt.Errorf("google userinfo: status %d", resp.StatusCode)
	}
	var ui struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ui); err != nil {
		return auth.OAuthUser{}, fmt.Errorf("google userinfo decode: %w", err)
	}
	return auth.OAuthUser{
		Sub:           ui.Sub,
		Email:         ui.Email,
		EmailVerified: ui.EmailVerified,
		Name:          ui.Name,
	}, nil
}
