package executor

import (
	"testing"
	"time"
)

func TestStateStorePersistsRunLifecycle(t *testing.T) {
	dir := t.TempDir()

	store, err := NewStateStore(dir)
	if err != nil {
		t.Fatalf("NewStateStore returned error: %v", err)
	}

	runID := "run-1"
	startedAt := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	if err := store.StartRun(runID, ".", 2, startedAt); err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if err := store.RecordTaskResult(runID, "a_test.go", "completed", "", "case", "", "", startedAt.Add(2*time.Second)); err != nil {
		t.Fatalf("RecordTaskResult success returned error: %v", err)
	}
	if err := store.RecordTaskResult(runID, "b_test.go", "failed", "boom", "case", "", "", startedAt.Add(4*time.Second)); err != nil {
		t.Fatalf("RecordTaskResult failure returned error: %v", err)
	}
	if err := store.CompleteRun(runID, "failed", startedAt.Add(5*time.Second)); err != nil {
		t.Fatalf("CompleteRun returned error: %v", err)
	}

	reloaded, err := NewStateStore(dir)
	if err != nil {
		t.Fatalf("reloading state store returned error: %v", err)
	}

	snapshot := reloaded.Snapshot()
	if snapshot.CurrentRun == nil {
		t.Fatal("CurrentRun = nil, want last run snapshot")
	}
	if snapshot.CurrentRun.Completed != 1 {
		t.Fatalf("Completed = %d, want 1", snapshot.CurrentRun.Completed)
	}
	if snapshot.CurrentRun.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", snapshot.CurrentRun.Failed)
	}
	if snapshot.CurrentRun.Status != "failed" {
		t.Fatalf("Status = %q, want %q", snapshot.CurrentRun.Status, "failed")
	}
	if len(snapshot.Runs) != 1 {
		t.Fatalf("Runs length = %d, want 1", len(snapshot.Runs))
	}
	if len(snapshot.CurrentRun.Tasks) != 2 {
		t.Fatalf("Tasks length = %d, want 2", len(snapshot.CurrentRun.Tasks))
	}
}
