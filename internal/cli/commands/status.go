package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// stateFile is the default location for migration state.
const stateFile = ".nep2midsence-state.json"

// MigrationState represents persisted migration progress.
type MigrationState struct {
	StartedAt   time.Time    `json:"started_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	TotalFiles  int          `json:"total_files"`
	Completed   int          `json:"completed"`
	Failed      int          `json:"failed"`
	Pending     int          `json:"pending"`
	CurrentFile string       `json:"current_file"`
	Runs        []RunRecord  `json:"runs"`
	Tasks       []TaskRecord `json:"tasks"`
}

// RunRecord represents a single migration run.
type RunRecord struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Status    string    `json:"status"` // "completed", "failed", "partial"
	Files     int       `json:"files"`
	Converted int       `json:"converted"`
	Errors    int       `json:"errors"`
}

// TaskRecord represents a single file migration task.
type TaskRecord struct {
	File   string `json:"file"`
	Status string `json:"status"` // "completed", "failed", "pending"
	Error  string `json:"error,omitempty"`
}

// loadState reads and parses the state file.
func loadState() (*MigrationState, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("状态文件不存在: %s (尚未运行过迁移)", stateFile)
		}
		return nil, fmt.Errorf("读取状态文件失败: %w", err)
	}
	var state MigrationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析状态文件失败: %w", err)
	}
	return &state, nil
}

// StatusCommand implements /status (alias /st).
type StatusCommand struct{}

func NewStatusCommand() *StatusCommand { return &StatusCommand{} }

func (c *StatusCommand) Name() string        { return "status" }
func (c *StatusCommand) Aliases() []string    { return []string{"st"} }
func (c *StatusCommand) Description() string  { return "查看当前迁移状态" }
func (c *StatusCommand) Usage() string        { return "/status" }

func (c *StatusCommand) Execute(args []string, cfg *config.Config) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	// Calculate progress percentage
	progress := 0.0
	if state.TotalFiles > 0 {
		progress = float64(state.Completed) / float64(state.TotalFiles) * 100
	}

	// Build progress bar (width 30)
	barWidth := 30
	filled := int(progress / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := fmt.Sprintf("[%s%s]",
		repeatChar('#', filled),
		repeatChar('-', barWidth-filled),
	)

	elapsed := state.UpdatedAt.Sub(state.StartedAt).Round(time.Second)

	fmt.Println("┌───────────────────────────────────────────────────────┐")
	fmt.Println("│                  迁移状态                              │")
	fmt.Println("├───────────────────────────────────────────────────────┤")
	fmt.Printf("│ 总文件数:    %-40d│\n", state.TotalFiles)
	fmt.Printf("│ 已完成:      %-40d│\n", state.Completed)
	fmt.Printf("│ 失败:        %-40d│\n", state.Failed)
	fmt.Printf("│ 待处理:      %-40d│\n", state.Pending)
	fmt.Printf("│ 当前文件:    %-40s│\n", truncate(state.CurrentFile, 40))
	fmt.Printf("│ 进度:        %-40s│\n", fmt.Sprintf("%s %.1f%%", bar, progress))
	fmt.Printf("│ 已用时间:    %-40s│\n", elapsed.String())
	fmt.Println("└───────────────────────────────────────────────────────┘")
	return nil
}

func repeatChar(ch byte, n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
