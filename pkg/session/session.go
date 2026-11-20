package session

import (
	"fmt"
	"mobius/pkg/agent"
	"mobius/pkg/agentctx"
	"mobius/pkg/guides"
	"sync"
	"time"
)

// Session represents an independent agent with its own persistent memory history.
type Session struct {
	ID        string
	Agent     *agent.Agent
	Context   *agentctx.ConversationContext
	CreatedAt time.Time

	mu        sync.RWMutex
	name      string
	started   bool
	updatedAt time.Time
}

// NewSession initializes an agent session with its own memory context.
func NewSession(id, name string, a *agent.Agent, systemPrompt ...string) *Session {
	prompt := ""
	if len(systemPrompt) > 0 {
		prompt = systemPrompt[0]
	}
	if prompt == "" {
		guidesPrompt := ""
		if gs, err := guides.LoadFromWorkSpace("config/guides.toml", "."); err == nil {
			guidesPrompt = gs.RenderSystemPrompt()
		}
		prompt = agentctx.BuildSystemPrompt("", guidesPrompt)
	}
	prompt += budgetNotice(a)

	convContext := agentctx.NewConversationContext(prompt)
	now := time.Now()
	return &Session{
		ID:        id,
		name:      name,
		Agent:     a,
		Context:   convContext,
		CreatedAt: now,
		updatedAt: now,
	}
}

func budgetNotice(a *agent.Agent) string {
	if a == nil {
		return ""
	}
	steps, cost, timeout := a.Limits()
	if steps <= 0 && cost <= 0 && timeout <= 0 {
		return ""
	}
	return fmt.Sprintf("\n\n# Limits\nHard stop from config/agent.toml: %d steps, $%.2f, %s. These limits apply to you. A question or a small review must be answered in the first few steps. Do not keep searching until the budget runs out.\n", steps, cost, timeout)
}

func (s *Session) Name() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.name
}

func (s *Session) Started() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}

func (s *Session) UpdatedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updatedAt
}

// MarkStarted sets the session name, marks it started, and updates the timestamp atomically.
func (s *Session) MarkStarted(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.name = name
	s.started = true
	s.updatedAt = time.Now()
}
