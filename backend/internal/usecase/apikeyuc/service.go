// Package apikeyuc is the application service for org-scoped API keys (FR-API-001/002).
// Create mints a secret shown once; ResolveByKey authenticates an inbound key on the
// request path. Domain-only deps (apikey + audit ports). It never imports another
// usecase package (clean layering).
package apikeyuc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/apikey"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// Deps wires the API-key service.
type Deps struct {
	Keys   apikey.APIKeyRepository
	Audit  audit.Writer // nil ⇒ no audit (tests)
	Logger *slog.Logger
	Now    func() time.Time
}

// Service implements API-key lifecycle + resolution.
type Service struct {
	keys   apikey.APIKeyRepository
	audit  audit.Writer
	logger *slog.Logger
	now    func() time.Time
}

// New builds the service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Service{keys: d.Keys, audit: d.Audit, logger: d.Logger, now: d.Now}
}

// Created is the result of Create: the stored key plus the one-time plaintext.
type Created struct {
	Key       apikey.APIKey
	Plaintext string // show once; never retrievable again
}

// Create mints and stores a new key for the org, returning the one-time plaintext.
// scopes is validated; an empty/invalid set is rejected.
func (s *Service) Create(ctx context.Context, orgID, name, createdBy string, scopes []apikey.Scope) (Created, error) {
	if name == "" {
		return Created{}, fmt.Errorf("%w: name required", domain.ErrValidation)
	}
	if len(scopes) == 0 {
		return Created{}, fmt.Errorf("%w: at least one scope required", domain.ErrValidation)
	}
	for _, sc := range scopes {
		if !sc.Valid() {
			return Created{}, fmt.Errorf("%w: invalid scope %q", domain.ErrValidation, sc)
		}
	}
	g, err := apikey.Generate()
	if err != nil {
		return Created{}, err
	}
	k := apikey.APIKey{
		ID:        uuidv7.New().String(),
		OrgID:     orgID,
		Prefix:    g.Prefix,
		KeyHash:   g.KeyHash,
		Name:      name,
		Scopes:    scopes,
		CreatedBy: createdBy,
		CreatedAt: s.now().UTC(),
	}
	if err := s.keys.Create(ctx, orgID, k); err != nil {
		return Created{}, fmt.Errorf("create api key: %w", err)
	}
	s.append(ctx, orgID, createdBy, audit.ActionAPIKeyCreate, k.ID, map[string]any{"name": name, "prefix": k.Prefix})
	return Created{Key: k, Plaintext: g.Plaintext}, nil
}

// List returns the org's keys (secrets never included).
func (s *Service) List(ctx context.Context, orgID string) ([]apikey.APIKey, error) {
	return s.keys.List(ctx, orgID)
}

// Revoke revokes one key the org owns.
func (s *Service) Revoke(ctx context.Context, orgID, id, actor string) error {
	if err := s.keys.Revoke(ctx, orgID, id); err != nil {
		return err
	}
	s.append(ctx, orgID, actor, audit.ActionAPIKeyRevoke, id, nil)
	return nil
}

// ResolveByKey authenticates an inbound key secret: hash → lookup → active check →
// best-effort last-used touch. Returns domain.ErrUnauthorized for any miss/revoked
// key so the auth path cannot distinguish "no such key" from "revoked".
func (s *Service) ResolveByKey(ctx context.Context, plaintext string) (apikey.APIKey, error) {
	if !apikey.LooksLikeKey(plaintext) {
		return apikey.APIKey{}, domain.ErrUnauthorized
	}
	hash := apikey.Hash(plaintext)
	k, err := s.keys.GetByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return apikey.APIKey{}, domain.ErrUnauthorized
		}
		return apikey.APIKey{}, err
	}
	if !k.Active() {
		return apikey.APIKey{}, domain.ErrUnauthorized
	}
	if err := s.keys.TouchLastUsed(ctx, hash); err != nil {
		s.logger.Warn("apikey touch last_used failed", "err", err)
	}
	return k, nil
}

// append writes a best-effort audit row (never surfaced to the caller).
func (s *Service) append(ctx context.Context, orgID, actor string, action audit.Action, targetID string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Append(ctx, audit.Entry{
		OrgID:       orgID,
		ActorUserID: actor,
		Action:      action,
		TargetType:  "api_key",
		TargetID:    targetID,
		Metadata:    meta,
		Severity:    audit.SeverityInfo,
	}); err != nil {
		s.logger.Warn("apikey audit append failed", "action", action, "err", err)
	}
}
