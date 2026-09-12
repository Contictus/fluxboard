// Package config parses 12-factor env config once at startup \(fail-fast, secrets redacted in String\(\)\)\. process configuration once at startup (12-factor:
// env only). Required keys missing => fail fast. Secrets never reach logs:
// String() redacts them (docs/10-INFRA-DEVOPS.md §2).
package config

import (
	"fmt"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

// Config is the fully-resolved runtime configuration. Construct it via Load;
// treat it as immutable afterwards. Fields tagged secret are masked by String().
type Config struct {
	AppEnv            string `envconfig:"APP_ENV" default:"dev"`
	HTTPAddr          string `envconfig:"HTTP_ADDR" default:":8080"`
	WorkerMetricsAddr string `envconfig:"WORKER_METRICS_ADDR" default:":8081"`

	// Database. app role for runtime queries; owner role only for migrations.
	DatabaseURL        string `envconfig:"DATABASE_URL" required:"true"`
	DatabaseURLMigrate string `envconfig:"DATABASE_URL_MIGRATE"`

	RedisAddr string `envconfig:"REDIS_ADDR" required:"true"`

	// Auth (Phase 1). PEM of the ES256 private key that signs access tokens.
	JWTPrivateKeyPEM string `envconfig:"JWT_PRIVATE_KEY_PEM" secret:"true"`
	TOTPEncKey       string `envconfig:"TOTP_ENC_KEY" secret:"true"`

	// Google OAuth2 (PKCE). Empty client id => the Google login routes 403.
	GoogleClientID     string `envconfig:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret string `envconfig:"GOOGLE_CLIENT_SECRET" secret:"true"`
	GoogleRedirectURL  string `envconfig:"GOOGLE_REDIRECT_URL" default:"http://localhost:8080/api/v1/auth/oauth/google/callback"`

	// Billing (Phase 4). StripeMode selects the gateway: "stub" (dev fake, no
	// external calls; webhook signatures are a shared-secret HMAC of the body)
	// or "live" (real stripe-go against the test/prod API). In stub mode
	// StripeWebhookSecret doubles as the HMAC key.
	StripeMode          string `envconfig:"STRIPE_MODE" default:"stub"`
	StripeSecretKey     string `envconfig:"STRIPE_SECRET_KEY" secret:"true"`
	StripeWebhookSecret string `envconfig:"STRIPE_WEBHOOK_SECRET" secret:"true"`
	StripePriceSeed     bool   `envconfig:"STRIPE_PRICE_SEED" default:"false"`

	// Object storage.
	MinIOEndpoint  string `envconfig:"MINIO_ENDPOINT"`
	MinIOAccessKey string `envconfig:"MINIO_ACCESS_KEY" secret:"true"`
	MinIOSecretKey string `envconfig:"MINIO_SECRET_KEY" secret:"true"`
	MinIOBucket    string `envconfig:"MINIO_BUCKET" default:"fluxboard"`
	MinIOUseSSL    bool   `envconfig:"MINIO_USE_SSL" default:"false"`

	// Mail (SMTP -> Mailpit in local dev).
	SMTPHost     string `envconfig:"SMTP_HOST"`
	SMTPPort     int    `envconfig:"SMTP_PORT" default:"1025"`
	SMTPUser     string `envconfig:"SMTP_USER"`
	SMTPPassword string `envconfig:"SMTP_PASSWORD" secret:"true"`
	SMTPFrom     string `envconfig:"SMTP_FROM" default:"no-reply@fluxboard.local"`

	WebOrigin string `envconfig:"WEB_ORIGIN" default:"http://localhost:3000"`
}

// Load reads configuration from the environment and returns it, or an error
// naming the first missing/invalid key.
func Load() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &c, nil
}

// IsProd reports whether the process runs in a production-shaped environment.
func (c *Config) IsProd() bool { return c.AppEnv == "prod" }

// String renders the config with every secret masked, safe to log at startup.
func (c *Config) String() string {
	var b strings.Builder
	b.WriteString("Config{")
	fmt.Fprintf(&b, "AppEnv=%s HTTPAddr=%s WorkerMetricsAddr=%s ", c.AppEnv, c.HTTPAddr, c.WorkerMetricsAddr)
	fmt.Fprintf(&b, "DatabaseURL=%s DatabaseURLMigrate=%s RedisAddr=%s ",
		redactDSN(c.DatabaseURL), redactDSN(c.DatabaseURLMigrate), c.RedisAddr)
	fmt.Fprintf(&b, "JWTPrivateKeyPEM=%s TOTPEncKey=%s ", mask(c.JWTPrivateKeyPEM), mask(c.TOTPEncKey))
	fmt.Fprintf(&b, "GoogleClientID=%s GoogleClientSecret=%s GoogleRedirectURL=%s ",
		c.GoogleClientID, mask(c.GoogleClientSecret), c.GoogleRedirectURL)
	fmt.Fprintf(&b, "StripeMode=%s StripeSecretKey=%s StripeWebhookSecret=%s StripePriceSeed=%t ",
		c.StripeMode, mask(c.StripeSecretKey), mask(c.StripeWebhookSecret), c.StripePriceSeed)
	fmt.Fprintf(&b, "MinIOEndpoint=%s MinIOAccessKey=%s MinIOSecretKey=%s MinIOBucket=%s MinIOUseSSL=%t ",
		c.MinIOEndpoint, mask(c.MinIOAccessKey), mask(c.MinIOSecretKey), c.MinIOBucket, c.MinIOUseSSL)
	fmt.Fprintf(&b, "SMTPHost=%s SMTPPort=%d SMTPUser=%s SMTPPassword=%s SMTPFrom=%s ",
		c.SMTPHost, c.SMTPPort, c.SMTPUser, mask(c.SMTPPassword), c.SMTPFrom)
	fmt.Fprintf(&b, "WebOrigin=%s}", c.WebOrigin)
	return b.String()
}

// mask returns a fixed redaction marker for non-empty secrets, "" for unset.
func mask(s string) string {
	if s == "" {
		return "\"\""
	}
	return "\"***\""
}

// redactDSN keeps the DSN shape but hides any embedded password.
func redactDSN(dsn string) string {
	if dsn == "" {
		return "\"\""
	}
	// postgres://user:pass@host/db -> postgres://user:***@host/db
	at := strings.LastIndex(dsn, "@")
	scheme := strings.Index(dsn, "://")
	if at == -1 || scheme == -1 {
		return "\"***\""
	}
	cred := dsn[scheme+3 : at]
	if colon := strings.Index(cred, ":"); colon != -1 {
		return dsn[:scheme+3] + cred[:colon] + ":***" + dsn[at:]
	}
	return dsn
}
