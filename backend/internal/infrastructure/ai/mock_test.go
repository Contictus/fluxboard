package ai

import (
	"context"
	"strings"
	"testing"

	domainai "github.com/mesutokul/fluxboard/backend/internal/domain/ai"
)

func TestMockParseDeterministic(t *testing.T) {
	m := NewMock()
	res, err := m.Complete(context.Background(), domainai.CompleteRequest{
		Kind: domainai.KindParse,
		User: "next week: finalise wireframes. kickoff design review; draft sprint demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != ModelMock {
		t.Fatalf("model = %q", res.Model)
	}
	for _, want := range []string{"finalise wireframes", "kickoff design review", "draft sprint demo"} {
		if !strings.Contains(res.Text, want) {
			t.Fatalf("parse missing %q in:\n%s", want, res.Text)
		}
	}
	if res.PromptTokens <= 0 || res.CompletionTokens <= 0 {
		t.Fatalf("tokens not estimated: %+v", res)
	}
	again, _ := m.Complete(context.Background(), domainai.CompleteRequest{
		Kind: domainai.KindParse,
		User: "next week: finalise wireframes. kickoff design review; draft sprint demo",
	})
	if again.Text != res.Text {
		t.Fatal("mock not deterministic")
	}
}

func TestMockParseEmpty(t *testing.T) {
	m := NewMock()
	res, err := m.Complete(context.Background(), domainai.CompleteRequest{Kind: domainai.KindParse, User: "   "})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "no tasks found") {
		t.Fatalf("empty parse = %q", res.Text)
	}
}

func TestMockClamp(t *testing.T) {
	m := NewMock()
	res, err := m.Complete(context.Background(), domainai.CompleteRequest{
		Kind: domainai.KindChat, User: "one two three four five six", MaxTokens: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Fields(res.Text)) > 3 {
		t.Fatalf("unclamped: %q", res.Text)
	}
}
