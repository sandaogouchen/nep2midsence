package commands

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// DryRunCommand implements /dry-run (alias /dr).
// Equivalent to /start --dry-run.
type DryRunCommand struct{}

func NewDryRunCommand() *DryRunCommand { return &DryRunCommand{} }

func (c *DryRunCommand) Name() string        { return "dry-run" }
func (c *DryRunCommand) Aliases() []string    { return []string{"dr"} }
func (c *DryRunCommand) Description() string  { return "模拟运行迁移（不实际写入文件）" }
func (c *DryRunCommand) Usage() string        { return "/dry-run [--dir <path>] [-j <workers>]" }

func (c *DryRunCommand) Execute(args []string, cfg *config.Config) error {
	// Prepend --dry-run and delegate to StartCommand
	startCmd := &StartCommand{}
	newArgs := append([]string{"--dry-run"}, args...)
	if err := startCmd.Execute(newArgs, cfg); err != nil {
		return fmt.Errorf("dry-run 失败: %w", err)
	}
	return nil
}
