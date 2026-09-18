// Minimal MCP surface (ADR-025, FR-AI-007). One endpoint, typed tools, same
// gates as HTTP: the org chain (HybridAuth, tenant, RBAC, scope) wraps the
// route, and every tool runs the aiuc pipeline (surface flag → meter →
// ledger → audit). POST ⇒ a read-scoped API key gets 403 from
// APIKeyScopeGuard even for read tools (v1 simplification; GET-based read
// tools are the follow-up). Concurrent-connection caps ride the existing
// plan rate limiter.
package handlers

import (
	"fmt"
	"net/http"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
)

// MCP tool names (closed set; unknown ⇒ 422).
const (
	MCPParse        = "ai.parse"
	MCPPlanDraft    = "ai.plan_draft"
	MCPDigest       = "ai.digest"
	MCPChat         = "ai.chat"
	MCPRisksScan    = "ai.risks_scan"
	MCPRisksList    = "ai.risks_list"
	MCPRisksDismiss = "ai.risks_dismiss"
)

type mcpReq struct {
	Tool   string         `json:"tool"`
	Params map[string]any `json:"params"`
}

// MCP dispatches one tool call.
func (h *AIHandlers) MCP(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req mcpReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	result, err := h.dispatchMCP(r, tc.OrgID, tc.UserID, req)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"tool": req.Tool, "result": result})
}

func (h *AIHandlers) dispatchMCP(r *http.Request, orgID, userID string, req mcpReq) (any, error) {
	ctx := r.Context()
	str := func(key string) string {
		v, _ := req.Params[key].(string)
		return v
	}
	strs := func(key string) []string {
		raw, _ := req.Params[key].([]any)
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	switch req.Tool {
	case MCPParse:
		out, err := h.svc.Parse(ctx, orgID, userID, str("input"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"run_id": out.RunID, "model": out.Model, "items": out.Items}, nil
	case MCPPlanDraft:
		key := str("idempotency_key")
		if key == "" {
			key = r.Header.Get("Idempotency-Key")
		}
		out, err := h.svc.PlanDraft(ctx, orgID, userID, str("brief"), key)
		if err != nil {
			return nil, err
		}
		return map[string]any{"run_id": out.RunID, "replayed": out.Replayed, "text": out.Text, "items": out.Items}, nil
	case MCPDigest:
		out, err := h.svc.Digest(ctx, orgID, userID, str("project_id"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"run_id": out.RunID, "text": out.Text, "stats": out.Stats}, nil
	case MCPChat:
		out, err := h.svc.Chat(ctx, orgID, userID, str("message"), strs("history"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"run_id": out.RunID, "text": out.Text}, nil
	case MCPRisksScan:
		risks, err := h.svc.ScanRisks(ctx, orgID, userID, str("project_id"))
		if err != nil {
			return nil, err
		}
		out := make([]riskResp, 0, len(risks))
		for i := range risks {
			out = append(out, toRiskResp(&risks[i]))
		}
		return map[string]any{"risks": out}, nil
	case MCPRisksList:
		risks, err := h.svc.ListRisks(ctx, orgID, userID, str("project_id"))
		if err != nil {
			return nil, err
		}
		out := make([]riskResp, 0, len(risks))
		for i := range risks {
			out = append(out, toRiskResp(&risks[i]))
		}
		return map[string]any{"risks": out}, nil
	case MCPRisksDismiss:
		if err := h.svc.DismissRisk(ctx, orgID, userID, str("task_id")); err != nil {
			return nil, err
		}
		return map[string]any{"dismissed": true}, nil
	default:
		return nil, fmt.Errorf("%w: unknown tool", domain.ErrValidation)
	}
}
