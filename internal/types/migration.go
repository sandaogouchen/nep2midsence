package types

type TaskStatus string

const (
	StatusPending TaskStatus = "pending"
	StatusRunning TaskStatus = "running"
	StatusDone    TaskStatus = "done"
	StatusFailed  TaskStatus = "failed"
	StatusSkipped TaskStatus = "skipped"
)

type MigrationTask struct {
	ID         string           `json:"id"`
	SourceFile string           `json:"source_file"`
	TargetFile string           `json:"target_file"`
	Analysis   *FullAnalysis    `json:"analysis"`
	Prompt     string           `json:"prompt"`
	Complexity string           `json:"complexity"`
	Status     TaskStatus       `json:"status"`
	Result     *MigrationResult `json:"result,omitempty"`
}
