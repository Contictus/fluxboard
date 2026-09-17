package tenantuc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
)

const (
	logoUploadTTL   = 15 * time.Minute
	logoDownloadTTL = 5 * time.Minute
	maxLogoBytes    = 5 << 20 // 5 MiB — same budget as avatars (useruc)
)

// allowedLogoTypes bounds the presigned upload to image content types.
var allowedLogoTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
}

// LogoStore issues presigned URLs for the org-logo object (MinIO). It is
// project.ObjectStore so the tenant usecase stays off the useruc port
// (usecase→usecase imports are forbidden); infrastructure satisfies both.
type LogoStore = project.ObjectStore

// RequestLogoUpload validates the intended upload and returns the object key
// plus a presigned PUT URL. The client uploads directly to MinIO, then records
// the key via PATCH /orgs/{id} {logo_key} (existing ADMIN-gated endpoint).
func (s *Service) RequestLogoUpload(ctx context.Context, orgID, contentType string, size int64) (key, url string, err error) {
	if s.logos == nil {
		return "", "", fmt.Errorf("%w: logo uploads unavailable", domain.ErrForbidden)
	}
	if _, ok := allowedLogoTypes[contentType]; !ok {
		return "", "", fmt.Errorf("%w: unsupported image type", domain.ErrValidation)
	}
	if size <= 0 || size > maxLogoBytes {
		return "", "", fmt.Errorf("%w: logo must be 1 byte..5 MiB", domain.ErrValidation)
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("logo key: %w", err)
	}
	key = fmt.Sprintf("org-logos/%s/%s", orgID, hex.EncodeToString(buf))
	url, err = s.logos.PresignPut(ctx, key, contentType, logoUploadTTL)
	if err != nil {
		return "", "", fmt.Errorf("logo upload url: %w", err)
	}
	return key, url, nil
}

// logoURL mints a short-lived read URL for a stored logo key. A missing/broken
// object must not break the org read — the caller drops the URL on error.
func (s *Service) logoURL(ctx context.Context, key string) string {
	if key == "" || s.logos == nil {
		return ""
	}
	url, err := s.logos.PresignGet(ctx, key, "logo", logoDownloadTTL)
	if err != nil {
		s.logger.WarnContext(ctx, "logo presign failed", "key", key, "error", err)
		return ""
	}
	return url
}

// checkLogoKey guards the PATCH logo_key passthrough: the key must live under
// this org's namespace (defense against pointing the org at someone else's
// object, e.g. another org's logo or a user's avatar).
func checkLogoKey(orgID, key string) error {
	if !strings.HasPrefix(key, fmt.Sprintf("org-logos/%s/", orgID)) {
		return fmt.Errorf("%w: logo key does not belong to org", domain.ErrValidation)
	}
	return nil
}
