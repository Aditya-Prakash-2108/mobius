package session

import (
	"fmt"
	"mobius/pkg/agent"
	"sync"
)

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	activeId string
}

type SessionInfo struct {
	ID       string
	Name     string
	IsActive bool
}

func NewManager(defaultSession *Session) *Manager {
	m := &Manager{
		sessions: make(map[string]*Session),
		activeId: defaultSession.ID,
	}
	// Add the default session into the map
	m.sessions[defaultSession.ID] = defaultSession
	return m
}

func (m *Manager) GetActive() (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, exists := m.sessions[m.activeId]
	if !exists {
		return nil, fmt.Errorf("no active session found")
	}
	return session, nil // the actual pointer so memory updates persist!
}

func (m *Manager) AddSession(session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = session
}

func (m *Manager) SwitchSession(sessionId string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, exists := m.sessions[sessionId]
	if !exists {
		return nil, fmt.Errorf("session '%s' not found", sessionId)
	}
	m.activeId = session.ID //  Updates the active session pointer!
	return session, nil
}

func (m *Manager) ListSession() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, SessionInfo{
			ID:       s.ID,
			Name:     s.Name(),
			IsActive: s.ID == m.activeId,
		})
	}
	return list
}

// CreateSession creates and switches to a new session
func (m *Manager) CreateSession(name string, a *agent.Agent) *Session {
	sess := NewSession(a.ThreadID(), name, a)
	m.mu.Lock()
	m.sessions[sess.ID] = sess
	m.activeId = sess.ID
	m.mu.Unlock()
	return sess
}

func (m *Manager) GetUnstarted() *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if !s.Started() {
			return s
		}
	}
	return nil
}
