package ai

import "testing"

func TestValidKind(t *testing.T) {
	for _, k := range []string{KindParse, KindPlanDraft, KindPlanApply, KindDigest, KindChat, KindRisk} {
		if !ValidKind(k) {
			t.Fatalf("ValidKind(%q) = false", k)
		}
	}
	if ValidKind("agent_takeover") {
		t.Fatal("ValidKind(unknown) = true")
	}
}

func TestSurfaceFor(t *testing.T) {
	if got := SurfaceFor(KindParse); got != FlagParse {
		t.Fatalf("SurfaceFor(parse) = %q", got)
	}
	if got := SurfaceFor(KindChat); got != FlagChat {
		t.Fatalf("SurfaceFor(chat) = %q", got)
	}
	if got := SurfaceFor("nope"); got != "" {
		t.Fatalf("SurfaceFor(unknown) = %q", got)
	}
}

func TestValidScore(t *testing.T) {
	for _, s := range []string{ScoreLow, ScoreMedium, ScoreHigh} {
		if !ValidScore(s) {
			t.Fatalf("ValidScore(%q) = false", s)
		}
	}
	if ValidScore("critical") {
		t.Fatal("ValidScore(unknown) = true")
	}
}
