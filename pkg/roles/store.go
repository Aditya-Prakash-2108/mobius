package roles

import (
	"context"
	"path/filepath"
	"sync"
)

// Store is a thread-safe handle on a roles directory.
// Reload swaps in a new catalog only after a successful load, so readers
// keep the previous snapshot when a file is invalid.
type Store struct {
	mu      sync.RWMutex
	dir     string
	catalog *Catalog
}

// Open loads dir and returns a store that can be reloaded while readers run.
func Open(ctx context.Context, dir string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	catalog, err := Load(ctx, abs)
	if err != nil {
		return nil, err
	}
	return &Store{dir: abs, catalog: catalog}, nil
}

// Reload reads the roles directory again.
// On failure the previous catalog stays in place.
func (s *Store) Reload(ctx context.Context) error {
	catalog, err := Load(ctx, s.dir)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.catalog = catalog
	s.mu.Unlock()
	return nil
}

// Role returns a copy of one role from the current snapshot.
func (s *Store) Role(id string) (Role, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.catalog == nil {
		return Role{}, false
	}
	return s.catalog.Role(id)
}

// Department returns a copy of one department from the current snapshot.
func (s *Store) Department(id string) (Department, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.catalog == nil {
		return Department{}, false
	}
	return s.catalog.Department(id)
}

// Roles returns a copy of every role in the current snapshot.
func (s *Store) Roles() []Role {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.catalog == nil {
		return []Role{}
	}
	return s.catalog.Roles()
}

// Departments returns a copy of every department in the current snapshot.
func (s *Store) Departments() []Department {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.catalog == nil {
		return []Department{}
	}
	return s.catalog.Departments()
}
