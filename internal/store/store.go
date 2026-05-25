package store

import (
	"maps"
	"sync"
	"time"
)

type Store interface {
	Add(string, Entry)
	Get(string) (Entry, bool)
	Delete(string) bool
	GetAll() map[string]Entry
}

type Entry struct {
	Value     string    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
	Deleted   bool      `json:"deleted"`
}

type MapStore struct {
	mu   sync.RWMutex
	Data map[string]Entry
}

func (m *MapStore) Add(key string, entry Entry) {
	m.mu.Lock()
	exists, ok := m.Data[key]

	// no op if already newer entry
	if exists.Timestamp.Before(entry.Timestamp) || !ok {
		m.Data[key] = entry
	}

	m.mu.Unlock()
}

func (m *MapStore) Get(key string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.Data[key]
	return value, ok
}

func (m *MapStore) Delete(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Data[key]; !ok {
		return false
	}
	delete(m.Data, key)
	return true
}

func (m *MapStore) GetAll() map[string]Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]Entry, len(m.Data))

	maps.Copy(out, m.Data)

	return out
}
