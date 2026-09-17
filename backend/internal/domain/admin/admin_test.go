package admin

import (
	"encoding/json"
	"strings"
	"testing"
)

// The tenant-detail surface embeds these domain structs verbatim; snake_case
// keys are the contract (ADR-022 follow-up, ADR-024). PascalCase on the wire
// is a defect — this pins the casing.
func TestDetailDTOs_SnakeCaseKeys(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want []string
	}{
		{"webhook", WebhookEvent{}, []string{`"event_id"`, `"processed_at"`}},
		{"override", EntitlementOverride{}, []string{`"org_id"`, `"created_by"`}},
		{"flag", FeatureFlag{}, []string{`"org_id"`, `"enabled"`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := json.Marshal(c.v)
			if err != nil {
				t.Fatal(err)
			}
			s := string(raw)
			for _, k := range c.want {
				if !strings.Contains(s, k) {
					t.Errorf("missing key %s in %s", k, s)
				}
			}
			for _, bad := range []string{`"OrgID"`, `"EventID"`, `"Flag"`, `"Key"`} {
				if strings.Contains(s, bad) {
					t.Errorf("PascalCase leak %s in %s", bad, s)
				}
			}
		})
	}
}
