package kvstore

import (
	"sync"
)

// Store represents a simple in-memory key-value store
type Store struct {
	mu   sync.RWMutex
	data map[string]string
}

// New creates a new key-value store
func New() *Store {
	return &Store{
		data: make(map[string]string),
	}
}

// Get retrieves a value by key
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.data[key]
	return value, exists
}

// Put stores a key-value pair
func (s *Store) Put(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Delete removes a key from the store
func (s *Store) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}
