// Package locks provides lightweight in-process locks for job isolation.
package locks

import "sync"

// Manager provides best-effort non-blocking locks for background job isolation.
type Manager struct {
	mu     sync.Mutex
	locked map[string]struct{}
}

// NewManager creates an empty lock manager.
func NewManager() *Manager {
	return &Manager{locked: make(map[string]struct{})}
}

// TryLock acquires a key if it is currently free and returns an unlock function.
func (m *Manager) TryLock(key string) (func(), bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.locked[key]; ok {
		return nil, false
	}
	m.locked[key] = struct{}{}

	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.locked, key)
	}, true
}
