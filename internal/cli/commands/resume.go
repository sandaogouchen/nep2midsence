package commands

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// ResumeCommand implements /resume (alias /r).
// Reads the state file, finds incomplete tasks, and resumes execution.
type ResumeCommand struct{}

func NewResumeCommand() *ResumeCommand { return &ResumeCommand{} }

func (c *ResumeCommand) Name() string        { return "resume" }
func (c *ResumeCommand) Aliases() []string    { return []string{"r"} }
func (c *ResumeCommand) Description() string  { return "恢复上次未完成的迁移" }
func (c *ResumeCommand) Usage() string        { return "/resume" }

func (c *ResumeCommand) Execute(args []string, cfg *config.Config) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	// Gather incomplete tasks
	var pending []TaskRecord
	for _, t := range state.Tasks {
		if t.Status == "pending" || t.Status == "failed" {
			pending = append(pending, t)
		}
	}

	if len(pending) == 0 {
		fmt.Println("没有需要恢复的任务，所有文件已处理完毕")
		return nil
	}

	fmt.Printf("找到 %d 个未完成的任务，正在恢复...\n", len(pending))
	fmt.Println()

	for i, task := range pending {
		fmt.Printf("  [%d/%d] %s", i+1, len(pending), task.File)
		if task.Status == "failed" {
			fmt.Printf(" (上次失败: %s)", task.Error)
		}
		fmt.Println()
	}
	fmt.Println()

	// Delegate to engine.
	// In a real implementation:
	//   engine := migrate.NewEngine(cfg)
	//   return engine.Resume(ctx, pending)
	fmt.Println("恢复执行中...")
	fmt.Println("[提示] 请实现 migrate.Engine.Resume 集成此处")
	return nil
}
