package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
)

// UserLookup loads a user by id for the platform-admin guard. Implemented by the
// postgres user repo (auth.UserRepository satisfies it); declared locally to keep
// the middleware off the usecase layer.
type UserLookup interface {
	GetByID(ctx context.Context, id string) (*auth.User, error)
}

// PlatformAdminGuard gates the /admin surface (FR-ADM-001, docs/11-SECURITY.md).
// It runs after Authenticate. Two conditions, both required:
//   - platform_role = admin
//   - TOTP enabled on the account — an admin account MUST have 2FA (the login flow
//     enforces the TOTP step for such accounts, so a live session already implies a
//     2FA-verified login; this is the "no admin route without 2FA" invariant,
//     6.7.4). An admin with 2FA disabled is refused until they enroll.
//
// An impersonation token may never reach the admin surface: it authenticates as the
// target org, not the platform, so ImpersonatedOrg must be empty here.
type PlatformAdminGuard struct {
	Users  UserLookup
	Logger *slog.Logger
}

// RequireAdmin is the middleware.
func (g *PlatformAdminGuard) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok || p.ImpersonatedOrg != "" {
			response.Error(w, domain.ErrForbidden)
			return
		}
		u, err := g.Users.GetByID(r.Context(), p.UserID)
		if err != nil {
			response.Error(w, domain.ErrForbidden)
			return
		}
		if u.PlatformRole != auth.PlatformRoleAdmin || !u.TOTPEnabled {
			response.Error(w, domain.ErrForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
