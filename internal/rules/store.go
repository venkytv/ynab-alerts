package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ObservedValue stores the captured value and when it was recorded.
type ObservedValue struct {
	Value      int64     `json:"value"`
	RecordedAt time.Time `json:"recorded_at"`
}

// Store persists observed variables to disk for reuse across runs.
type Store struct {
	path   string
	values map[string]ObservedValue
	mu     sync.Mutex
}

// NewStore returns a Store persisted at path.
func NewStore(path string) (*Store, error) {
	s := &Store{
		path:   path,
		values: map[string]ObservedValue{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.values)
}

// Snapshot returns a copy of stored variables.
func (s *Store) Snapshot() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]int64, len(s.values))
	for k, v := range s.values {
		out[k] = v.Value
	}
	return out
}

// Get returns an observed value.
func (s *Store) Get(name string) (ObservedValue, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[name]
	return v, ok
}

// Set writes an observed value and persists it.
func (s *Store) Set(name string, val ObservedValue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.values[name] = val
	data, err := json.MarshalIndent(s.values, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
