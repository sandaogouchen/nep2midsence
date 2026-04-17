package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const stateFileName = ".nep2midsence-state.json"

// TaskSnapshot captures the persisted state of a single migrated file.
type TaskSnapshot struct {
	File       string    `json:"file"`
	TargetFile string    `json:"target_file,omitempty"`
	Kind       string    `json:"kind,omitempty"`        // case | helper
	SourceHash string    `json:"source_hash,omitempty"` // sha256(file bytes)
	Status     string    `json:"status"`                // running | completed | failed | skipped
	Error      string    `json:"error,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// RunSnapshot captures a single migration or analysis run.
type RunSnapshot struct {
	ID         string         `json:"id"`
	Dir        string         `json:"dir"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	EndedAt    time.Time      `json:"ended_at,omitempty"`
	TotalFiles int            `json:"total_files"`
	Completed  int            `json:"completed"`
	Failed     int            `json:"failed"`
	Pending    int            `json:"pending"`
	CurrentFile string        `json:"current_file,omitempty"`
	Tasks      []TaskSnapshot `json:"tasks"`
}

// StateSnapshot is the shared persisted format used by the TUI for status/history views.
type StateSnapshot struct {
	CurrentRun *RunSnapshot  `json:"current_run,omitempty"`
	Runs       []RunSnapshot `json:"runs"`
}

// StateStore persists migration state to disk for resumability and status/history views.
type StateStore struct {
	mu       sync.RWMutex
	filePath string
	state    StateSnapshot
}

// NewStateStore creates or loads a state store at the given directory.
func NewStateStore(dir string) (*StateStore, error) {
	fp := filepath.Join(dir, stateFileName)
	store := &StateStore{
		filePath: fp,
		state: StateSnapshot{
			Runs: make([]RunSnapshot, 0),
		},
	}

	data, err := os.ReadFile(fp)
	if err == nil {
		if err := json.Unmarshal(data, &store.state); err != nil {
			return nil, fmt.Errorf("parse state file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	return store, nil
}

// StartRun creates a new run and persists it immediately.
func (s *StateStore) StartRun(runID, dir string, totalFiles int, startedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	run := RunSnapshot{
		ID:         runID,
		Dir:        dir,
		Status:     "running",
		StartedAt:  startedAt,
		UpdatedAt:  startedAt,
		TotalFiles: totalFiles,
		Pending:    totalFiles,
		Tasks:      make([]TaskSnapshot, 0, totalFiles),
	}

	s.state.Runs = append(s.state.Runs, run)
	s.state.CurrentRun = &s.state.Runs[len(s.state.Runs)-1]

	return s.saveLocked()
}

// RecordTaskResult updates the active run with a completed task result.
func (s *StateStore) RecordTaskResult(runID, file, status, errMsg, kind, sourceHash, targetFile string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	run := s.findRunLocked(runID)
	if run == nil {
		return fmt.Errorf("unknown run id %q", runID)
	}

	status = normalizeTaskStatus(status)

	replaced := false
	for i := range run.Tasks {
		if run.Tasks[i].File == file {
			run.Tasks[i].Status = status
			run.Tasks[i].Error = errMsg
			run.Tasks[i].Kind = kind
			run.Tasks[i].SourceHash = sourceHash
			run.Tasks[i].TargetFile = targetFile
			run.Tasks[i].UpdatedAt = updatedAt
			replaced = true
			break
		}
	}
	if !replaced {
		run.Tasks = append(run.Tasks, TaskSnapshot{
			File:       file,
			TargetFile: targetFile,
			Kind:       kind,
			SourceHash: sourceHash,
			Status:     status,
			Error:      errMsg,
			UpdatedAt:  updatedAt,
		})
	}

	run.Completed = countTasksAny(run.Tasks, "completed", "skipped")
	run.Failed = countTasksAny(run.Tasks, "failed")
	run.Pending = max(0, run.TotalFiles-run.Completed-run.Failed)
	run.CurrentFile = file
	run.UpdatedAt = updatedAt

	if s.state.CurrentRun != nil && s.state.CurrentRun.ID == runID {
		*s.state.CurrentRun = *run
	}

	return s.saveLocked()
}

// LatestCompletedTask returns the most recent completed/skipped task for the file.
func (s *StateStore) LatestCompletedTask(file string) *TaskSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.state.Runs) - 1; i >= 0; i-- {
		run := s.state.Runs[i]
		for j := len(run.Tasks) - 1; j >= 0; j-- {
			t := run.Tasks[j]
			if t.File != file {
				continue
			}
			if t.Status == "completed" || t.Status == "skipped" {
				cp := t
				return &cp
			}
		}
	}
	return nil
}

// IsUpToDate reports whether the file has been migrated and the source hash is unchanged.
// If targetFile is non-empty, it must exist on disk.
func (s *StateStore) IsUpToDate(file, sourceHash, targetFile string) bool {
	if strings.TrimSpace(file) == "" || strings.TrimSpace(sourceHash) == "" {
		return false
	}
	latest := s.LatestCompletedTask(file)
	if latest == nil {
		return false
	}
	if latest.SourceHash == "" || latest.SourceHash != sourceHash {
		return false
	}
	if strings.TrimSpace(targetFile) != "" {
		if _, err := os.Stat(targetFile); err != nil {
			return false
		}
	}
	return true
}

// CompleteRun marks the run as finished.
func (s *StateStore) CompleteRun(runID, status string, endedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	run := s.findRunLocked(runID)
	if run == nil {
		return fmt.Errorf("unknown run id %q", runID)
	}

	run.Status = status
	run.EndedAt = endedAt
	run.UpdatedAt = endedAt
	run.Pending = max(0, run.TotalFiles-run.Completed-run.Failed)

	if s.state.CurrentRun != nil && s.state.CurrentRun.ID == runID {
		*s.state.CurrentRun = *run
	}

	return s.saveLocked()
}

// Snapshot returns a copy of the current persisted state.
func (s *StateStore) Snapshot() StateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw, _ := json.Marshal(s.state)
	var cp StateSnapshot
	_ = json.Unmarshal(raw, &cp)
	return cp
}

func (s *StateStore) findRunLocked(runID string) *RunSnapshot {
	for i := range s.state.Runs {
		if s.state.Runs[i].ID == runID {
			return &s.state.Runs[i]
		}
	}
	return nil
}

func (s *StateStore) saveLocked() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0o644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

func countTasksAny(tasks []TaskSnapshot, statuses ...string) int {
	if len(statuses) == 0 {
		return 0
	}
	want := make(map[string]struct{}, len(statuses))
	for _, st := range statuses {
		want[st] = struct{}{}
	}
	count := 0
	for _, task := range tasks {
		if _, ok := want[task.Status]; ok {
			count++
		}
	}
	return count
}

func normalizeTaskStatus(status string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	switch status {
	case "completed", "failed", "skipped", "running":
		return status
	default:
		// Backward-compatible fallback: treat unknown status as failed.
		return "failed"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
