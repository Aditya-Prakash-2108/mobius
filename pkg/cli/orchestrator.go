package cli

import (
	"context"
	"fmt"

	"mobius/pkg/agent"
	"mobius/pkg/events"
	"mobius/pkg/llm"
	"mobius/pkg/orchestrator"
	"mobius/pkg/roles"
	"mobius/pkg/session"
	"mobius/pkg/tools"
)

// App bundles shared CLI wiring for workspaces and sessions.
type App struct {
	Sessions   *session.Manager
	Workspaces *orchestrator.WorkspaceRegistry
	RoleStore  *roles.Store
	BaseTools  *tools.Registry
	Config     *llm.Config
	Events     events.EventStore
}

// NewApp creates session/workspace wiring for interactive and one-shot CLI.
func NewApp(sm *session.Manager, ws *orchestrator.WorkspaceRegistry, store *roles.Store, base *tools.Registry, cfg *llm.Config, ev events.EventStore) *App {
	return &App{
		Sessions:   sm,
		Workspaces: ws,
		RoleStore:  store,
		BaseTools:  base,
		Config:     cfg,
		Events:     ev,
	}
}

// AgentFactory builds agents using the shared event store.
func (a *App) AgentFactory() orchestrator.AgentFactory {
	return func(registry *tools.Registry, model orchestrator.ModelSpec) (*agent.Agent, error) {
		return agent.NewAgent(model.Provider, registry, model.Name, model.PromptPrice, model.CompletionPrice, a.Events)
	}
}

// ActiveModelSpec reads prices for the active configured model.
func (a *App) ActiveModelSpec(provider llm.Provider) orchestrator.ModelSpec {
	pCost, cCost := a.Config.GetPrices(a.Config.ActiveModel)
	return orchestrator.ModelSpec{
		Name:            a.Config.ActiveModel,
		Provider:        provider,
		PromptPrice:     pCost,
		CompletionPrice: cCost,
	}
}

// WorkspaceForSession returns the workspace for a chat session id.
func (a *App) WorkspaceForSession(sess *session.Session) (*orchestrator.Workspace, error) {
	ws, ok := a.Workspaces.Get(sess.ID)
	if !ok {
		return nil, fmt.Errorf("no workspace for session %s", sess.ID)
	}
	return ws, nil
}

// AttachLineHook wires worker/orchestrator output to the session line hook pattern.
func AttachLineHook(ag *agent.Agent, hook agent.LineHook) {
	if hook != nil {
		ag.SetLineHook(hook)
	}
}

// DispatchUserInput routes one prompt through the workspace orchestrator.
func (a *App) DispatchUserInput(ctx context.Context, sess *session.Session, input string, sync bool) (orchestrator.DispatchResult, error) {
	ws, err := a.WorkspaceForSession(sess)
	if err != nil {
		return orchestrator.DispatchResult{}, err
	}
	mode := orchestrator.DispatchAsync
	if sync {
		mode = orchestrator.DispatchSync
	}
	return ws.Handle(ctx, input, mode)
}
