package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// StateStore persists migration state to disk for resumability
type StateStore struct {
	mu       sync.RWMutex
	filePath string
	state    *MigrationState
}

// MigrationState represents the persisted state of a migration run
type MigrationState struct {
	Tasks   map[string]*types.MigrationTask `json:"tasks"`
	Started string                          `json:"started"`
	Updated string                          `json:"updated"`
}

// NewStateStore creates or loads a state store at the given path
func NewStateStore(dir string) (*StateStore, error) {
	fp := filepath.Join(dir, ".nep2midsence-state.json")
	store := &StateStore{
		filePath: fp,
		state: &MigrationState{
			Tasks: make(map[string]*types.MigrationTask),
		},
	}

	// Try to load existing state
	if data, err := os.ReadFile(fp); err == nil {
		var existing MigrationState
		if err := json.Unmarshal(data, &existing); err == nil {
			store.state = &existing
		}
	}

	return store, nil
}

// Get retrieves a task by ID
func (s *StateStore) Get(id string) *types.MigrationTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Tasks[id]
}

// Set stores a task
func (s *StateStore) Set(task *types.MigrationTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Tasks[task.ID] = task
}

// Save writes state to disk
func (s *StateStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// AllTasks returns all stored tasks
func (s *StateStore) AllTasks() []*types.MigrationTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*types.MigrationTask, 0, len(s.state.Tasks))
	for _, t := range s.state.Tasks {
		tasks = append(tasks, t)
	}
	return tasks
}
