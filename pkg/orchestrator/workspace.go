package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"mobius/pkg/roles"
	"mobius/pkg/session"
	"mobius/pkg/tools"
)

// Workspace is an isolated per-chat orchestrator and team runtime.
type Workspace struct {
	chatID       string
	orchestrator *session.Session
	manager      *Manager
	ctx          context.Context
	cancel       context.CancelFunc
	model        ModelSpec
	factory      AgentFactory
}

// WorkspaceRegistry maps chat/session ids to workspaces.
type WorkspaceRegistry struct {
	mu sync.Mutex
	m  map[string]*Workspace
}

// NewWorkspaceRegistry initializes an empty registry.
func NewWorkspaceRegistry() *WorkspaceRegistry {
	return &WorkspaceRegistry{m: make(map[string]*Workspace)}
}

// Get returns a workspace by chat id.
func (r *WorkspaceRegistry) Get(chatID string) (*Workspace, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.m[chatID]
	return w, ok
}

// Register adds a workspace for a chat id.
func (r *WorkspaceRegistry) Register(chatID string, w *Workspace) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[chatID] = w
}

// Remove deletes a workspace entry.
func (r *WorkspaceRegistry) Remove(chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, chatID)
}

// NewWorkspace builds an isolated orchestrator session and optional default team.
func NewWorkspace(parent context.Context, chatID string, roleStore *roles.Store, baseTools *tools.Registry, model ModelSpec, factory AgentFactory) (*Workspace, error) {
	if parent == nil {
		parent = context.Background()
	}
	if roleStore == nil {
		return nil, fmt.Errorf("role store is required")
	}
	if factory == nil {
		return nil, fmt.Errorf("agent factory is required")
	}
	orchTools := tools.NewRegistry()
	orchAgent, err := factory(orchTools, model)
	if err != nil {
		return nil, fmt.Errorf("orchestrator agent: %w", err)
	}
	orchSess := session.NewSession(orchAgent.ThreadID(), "orchestrator", orchAgent, orchestratorSystemPrompt)
	orchSess.MarkStarted("orchestrator")

	ctx, cancel := context.WithCancel(parent)
	w := &Workspace{
		chatID:       chatID,
		orchestrator: orchSess,
		manager:      newManager(roleStore, baseTools, model, factory),
		ctx:          ctx,
		cancel:       cancel,
		model:        model,
		factory:      factory,
	}

	if _, ok := roleStore.Department("engineering"); ok {
		if err := w.manager.CreateTeam("engineering"); err != nil {
			cancel()
			return nil, err
		}
	}
	return w, nil
}

// ChatID returns the workspace chat identifier.
func (w *Workspace) ChatID() string { return w.chatID }

// OrchestratorSession is the user-visible chat session (zero tools).
func (w *Workspace) OrchestratorSession() *session.Session { return w.orchestrator }

// Manager returns the team manager.
func (w *Workspace) Manager() *Manager { return w.manager }

// Context returns the cancellable worker context.
func (w *Workspace) Context() context.Context { return w.ctx }

// SetModel updates orchestrator and all workers.
func (w *Workspace) SetModel(model ModelSpec) {
	w.model = model
	w.orchestrator.Agent.SetModel(model.Name, model.Provider, model.PromptPrice, model.CompletionPrice)
	w.manager.SetModel(model)
}

// Close cancels background workers for this chat.
func (w *Workspace) Close() {
	if w.cancel != nil {
		w.cancel()
	}
}
