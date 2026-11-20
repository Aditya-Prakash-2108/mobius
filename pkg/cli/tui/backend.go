package tui

import (
	"context"
)

// ChatSnapshot is one selectable chat session.
type ChatSnapshot struct {
	ID   string
	Name string
}

// TeamSnapshot is a department with ordered roles for mention/view menus.
type TeamSnapshot struct {
	ID    string
	Name  string
	Roles []RoleSnapshot
}

// RoleSnapshot is one mentionable worker role.
type RoleSnapshot struct {
	ID   string
	Name string
}

// ModelOption is a selectable LLM id.
type ModelOption struct {
	ID      string
	Current bool
}

// ContextSnapshot is sidebar state from the backend.
type ContextSnapshot struct {
	Breakdown ContextBreakdown
	Ledger    BudgetSnapshot
}

// DispatchHandle tracks an in-flight user dispatch.
type DispatchHandle struct {
	ChatID string
	RunID  string
	Kind   string
	Reply  string
	TeamID string
	RoleID string
	Done   <-chan DispatchDone
	Trace  []string
}

// DispatchDone is the worker completion payload.
type DispatchDone struct {
	Reply string
	Err   error
}

// Backend is implemented by pkg/cli without importing it from tui tests.
type Backend interface {
	ActiveChat() (ChatSnapshot, error)
	ListChats() []ChatSnapshot
	ListModels() ([]ModelOption, error)
	Catalog(chatID string) []TeamSnapshot
	ReloadRoles(ctx context.Context, chatID string) error
	InitWorkspace(ctx context.Context, chatID string) (string, error)
	TeamsText(chatID string) string
	NewChat(ctx context.Context) (ChatSnapshot, error)
	SwitchChat(chatID string) error
	SetModel(chatID, model string) error
	CurrentModel(chatID string) string
	ContextFor(chatID, viewRoleID string) ContextSnapshot
	Dispatch(ctx context.Context, chatID, input, runID string) (DispatchHandle, error)
	Subscribe(chatID string, send func(LiveEvent)) context.CancelFunc
	SessionID() string
}
