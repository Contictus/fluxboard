// Package ai implements the infrastructure side of the governed AI layer
// (ADR-025, FR-AI-001..008): the ai.Provider port. Mock is the default —
// deterministic, keyless, boot-safe (MinIO pattern: degraded ⇒ safe, never
// fatal). Live Claude/Azure failover plugs the same port later; the usecase
// never branches on provider identity.
package ai

import (
	"context"
	"fmt"
	"strings"

	domainai "github.com/mesutokul/fluxboard/backend/internal/domain/ai"
)

// ModelMock is the model tag persisted on mock runs (audit + metering).
const ModelMock = "mock"

// Mock is a deterministic provider: no network, no keys, pinned outputs.
// Parse splits the input into task clauses; every other kind returns a
// templated draft referencing the input. Token counts are word-based
// estimates (stable across runs, hence metering-testable).
type Mock struct{}

// NewMock builds a Mock.
func NewMock() *Mock { return &Mock{} }

// Compile-time port check.
var _ domainai.Provider = (*Mock)(nil)

// Complete implements ai.Provider.
func (m *Mock) Complete(_ context.Context, req domainai.CompleteRequest) (domainai.CompleteResponse, error) {
	var text string
	switch req.Kind {
	case domainai.KindParse:
		text = parseMock(req.User)
	case domainai.KindPlanDraft:
		text = fmt.Sprintf("Plan draft from brief (%d chars):\n- Phase 1: scope (%s)\n- Phase 2: build\n- Phase 3: verify",
			len(req.User), excerpt(req.User, 60))
	case domainai.KindDigest:
		text = fmt.Sprintf("Status digest (%d chars of project data):\n- Accomplished: see input\n- Blockers: none stated\n- Next: continue planned work",
			len(req.User))
	case domainai.KindRisk:
		text = "Risk review: overdue and overcommit signals checked; details in signals payload."
	case domainai.KindChat:
		if len(req.History) > 0 {
			text = fmt.Sprintf("Reply (turn %d): %s", len(req.History)+1, excerpt(req.User, 120))
		} else {
			text = fmt.Sprintf("Reply: %s", excerpt(req.User, 120))
		}
	default:
		text = fmt.Sprintf("Draft: %s", excerpt(req.User, 120))
	}
	if req.MaxTokens > 0 {
		text = clampWords(text, req.MaxTokens)
	}
	return domainai.CompleteResponse{
		Text:             text,
		Model:            ModelMock,
		PromptTokens:     countWords(req.System) + countWords(req.User),
		CompletionTokens: countWords(text),
	}, nil
}

// parseMock turns free text into "- task" lines: split on newlines, then on
// ". ", "; " boundaries; first 8 non-empty clauses win. Deterministic.
func parseMock(input string) string {
	norm := strings.ReplaceAll(input, "\r\n", "\n")
	var clauses []string
	for _, line := range strings.Split(norm, "\n") {
		for _, part := range strings.Split(line, "; ") {
			for _, s := range strings.Split(part, ". ") {
				s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
				if s != "" {
					clauses = append(clauses, s)
				}
			}
		}
	}
	if len(clauses) == 0 {
		return "- (no tasks found)"
	}
	if len(clauses) > 8 {
		clauses = clauses[:8]
	}
	var b strings.Builder
	for _, c := range clauses {
		b.WriteString("- " + c + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// excerpt returns the first n runes of s (no broken UTF-8, no invented text).
func excerpt(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// clampWords caps text at max words (cost bound, ADR-025).
func clampWords(s string, max int) string {
	w := strings.Fields(s)
	if len(w) <= max {
		return s
	}
	return strings.Join(w[:max], " ")
}

// countWords is the stable token estimate (mock metering).
func countWords(s string) int { return len(strings.Fields(s)) }
