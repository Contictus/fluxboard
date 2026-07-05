package redisx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
)

// OAuthStateStore keeps per-flow PKCE state in Redis (docs/04-AUTH.md §4). State
// is single-use: Take reads and deletes atomically (GETDEL) so a callback cannot
// be replayed.
type OAuthStateStore struct {
	rdb *redis.Client
}

// NewOAuthStateStore builds an OAuthStateStore over the given client.
func NewOAuthStateStore(rdb *redis.Client) *OAuthStateStore { return &OAuthStateStore{rdb: rdb} }

var _ auth.OAuthStateStore = (*OAuthStateStore)(nil)

func oauthKey(state string) string { return "oauth:" + state }

func (s *OAuthStateStore) Save(ctx context.Context, state string, v auth.OAuthState, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("oauth state marshal: %w", err)
	}
	return s.rdb.Set(ctx, oauthKey(state), b, ttl).Err()
}

func (s *OAuthStateStore) Take(ctx context.Context, state string) (auth.OAuthState, error) {
	b, err := s.rdb.GetDel(ctx, oauthKey(state)).Bytes()
	if errors.Is(err, redis.Nil) {
		return auth.OAuthState{}, domain.ErrNotFound
	}
	if err != nil {
		return auth.OAuthState{}, fmt.Errorf("oauth state take: %w", err)
	}
	var v auth.OAuthState
	if err := json.Unmarshal(b, &v); err != nil {
		return auth.OAuthState{}, fmt.Errorf("oauth state unmarshal: %w", err)
	}
	return v, nil
}
