// Package mailer sends the transactional auth emails (verify, password reset)
// over SMTP — Mailpit in local dev, a real relay in prod. It implements
// authuc.Mailer. When no SMTP host is configured it falls back to logging the
// link so the flow stays testable without a mail server (docs/04-AUTH.md).
package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"net/url"
	"strings"
)

// Config holds the SMTP endpoint and the message envelope.
type Config struct {
	Host      string // empty => dev log-only mode
	Port      int
	Username  string
	Password  string
	From      string
	WebOrigin string // base URL for building the click-through links
}

// Mailer sends mail per Config. Safe for concurrent use.
type Mailer struct {
	cfg    Config
	logger *slog.Logger
}

// New builds a Mailer. A nil logger is tolerated (falls back to slog.Default).
func New(cfg Config, logger *slog.Logger) *Mailer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Mailer{cfg: cfg, logger: logger}
}

// SendEmailVerify emails the 24h verification link (docs/04-AUTH.md §2).
func (m *Mailer) SendEmailVerify(ctx context.Context, to, rawToken string) error {
	link := m.link("/verify-email", rawToken)
	return m.send(ctx, to, "Verify your Fluxboard email",
		"Welcome to Fluxboard. Confirm your address:\n\n"+link+
			"\n\nThis link expires in 24 hours. If you did not sign up, ignore this email.")
}

// SendPasswordReset emails the 1h reset link (docs/04-AUTH.md §2).
func (m *Mailer) SendPasswordReset(ctx context.Context, to, rawToken string) error {
	link := m.link("/reset-password", rawToken)
	return m.send(ctx, to, "Reset your Fluxboard password",
		"A password reset was requested for your account:\n\n"+link+
			"\n\nThis link expires in 1 hour. If you did not request it, ignore this email.")
}

// SendInvitation emails the org-invite click-through (docs/05-TENANCY-RBAC.md
// §5). The link is path-style ({WebOrigin}/invite/{token}) to match the sitemap.
func (m *Mailer) SendInvitation(ctx context.Context, to, orgName, rawToken string) error {
	base := strings.TrimRight(m.cfg.WebOrigin, "/")
	link := base + "/invite/" + url.PathEscape(rawToken)
	org := orgName
	if org == "" {
		org = "an organization"
	}
	return m.send(ctx, to, "You've been invited to "+org+" on Fluxboard",
		"You have been invited to join "+org+" on Fluxboard:\n\n"+link+
			"\n\nThis invitation expires in 7 days.")
}

// link builds {WebOrigin}{path}?token=<raw>.
func (m *Mailer) link(path, rawToken string) string {
	base := strings.TrimRight(m.cfg.WebOrigin, "/")
	return base + path + "?token=" + url.QueryEscape(rawToken)
}

// send delivers one plaintext message, or logs it in dev (no SMTP host).
func (m *Mailer) send(_ context.Context, to, subject, body string) error {
	if m.cfg.Host == "" {
		// Dev mode: no relay configured. Log the body so the link is testable.
		// Never logs token material in prod because prod configures a host.
		m.logger.Info("mailer dev fallback (SMTP not configured)", "to", to, "subject", subject, "body", body)
		return nil
	}
	addr := net.JoinHostPort(m.cfg.Host, fmt.Sprintf("%d", m.cfg.Port))
	msg := m.buildMessage(to, subject, body)

	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}
	if err := smtp.SendMail(addr, auth, m.cfg.From, []string{to}, msg); err != nil {
		return fmt.Errorf("mailer send: %w", err)
	}
	return nil
}

// buildMessage assembles a minimal RFC 5322 message.
func (m *Mailer) buildMessage(to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", m.cfg.From)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
