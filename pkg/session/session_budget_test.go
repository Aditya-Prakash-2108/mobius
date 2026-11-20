package session

import (
	"strings"
	"testing"
)

func TestSessionIncludesAgentBudget(t *testing.T) {
	chdirModuleRoot(t)
	ag := newTestAgent(t)
	steps, _, _ := ag.Limits()
	if steps != 8 {
		t.Fatalf("config/agent.toml max_steps = %d, want 8", steps)
	}

	sess := NewSession("s1", "Code Review", ag, "You are Code Review.")
	msgs := sess.Context.Messages()
	if len(msgs) == 0 {
		t.Fatal("expected a system message")
	}
	if !strings.Contains(msgs[0].Content, "You are Code Review.") {
		t.Fatalf("role text missing: %s", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "Hard stop from config/agent.toml: 8 steps") {
		t.Fatalf("budget notice missing: %s", msgs[0].Content)
	}
}
