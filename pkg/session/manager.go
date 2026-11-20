package session

import (
	"fmt"
	"mobius/pkg/agent"
	"sync"
)

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	order    []string
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
		order:    []string{defaultSession.ID},
		activeId: defaultSession.ID,
	}
	m.sessions[defaultSession.ID] = defaultSession
	return m
}

func (m *Manager) Get(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, exists := m.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session '%s' not found", id)
	}
	return session, nil
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
	if _, exists := m.sessions[session.ID]; !exists {
		m.order = append(m.order, session.ID)
	}
	m.sessions[session.ID] = session
}

// Remove drops a session. The returned session is the one left active.
func (m *Manager) Remove(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return nil, fmt.Errorf("session '%s' not found", id)
	}
	delete(m.sessions, id)
	order := make([]string, 0, len(m.order))
	removedAt := 0
	for i, existing := range m.order {
		if existing == id {
			removedAt = i
			continue
		}
		order = append(order, existing)
	}
	m.order = order
	if m.activeId != id {
		if current, ok := m.sessions[m.activeId]; ok {
			return current, nil
		}
	}
	if len(m.order) == 0 {
		m.activeId = ""
		return nil, nil
	}
	if removedAt >= len(m.order) {
		removedAt = len(m.order) - 1
	}
	m.activeId = m.order[removedAt]
	return m.sessions[m.activeId], nil
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
	list := make([]SessionInfo, 0, len(m.order))
	for _, id := range m.order {
		s, ok := m.sessions[id]
		if !ok {
			continue
		}
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
	if _, exists := m.sessions[sess.ID]; !exists {
		m.order = append(m.order, sess.ID)
	}
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
