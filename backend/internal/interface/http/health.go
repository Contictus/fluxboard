package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Pinger is anything whose reachability gates readiness (pgxpool, redis client).
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health serves liveness and readiness probes (docs/10-INFRA-DEVOPS.md §6).
// Liveness (/healthz) says only "process is up" so a dependency blip never
// triggers a restart loop. Readiness (/readyz) gates traffic on DB + Redis.
type Health struct {
	DB    Pinger
	Redis Pinger
}

// Live handles /healthz: always 200 if the process can serve.
func (h Health) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready handles /readyz: 200 only when every dependency answers, else 503 with
// the per-check results (fail closed).
func (h Health) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{}
	ok := true
	for name, dep := range map[string]Pinger{"database": h.DB, "redis": h.Redis} {
		if dep == nil {
			checks[name] = "unconfigured"
			ok = false
			continue
		}
		if err := dep.Ping(ctx); err != nil {
			checks[name] = "down: " + err.Error()
			ok = false
		} else {
			checks[name] = "ok"
		}
	}

	status := http.StatusOK
	if !ok {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{"ready": ok, "checks": checks})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
