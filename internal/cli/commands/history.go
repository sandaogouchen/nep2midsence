package commands

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// HistoryCommand implements /history (alias /hi).
// Lists past migration runs from the state file.
type HistoryCommand struct{}

func NewHistoryCommand() *HistoryCommand { return &HistoryCommand{} }

func (c *HistoryCommand) Name() string        { return "history" }
func (c *HistoryCommand) Aliases() []string    { return []string{"hi"} }
func (c *HistoryCommand) Description() string  { return "查看历史迁移记录" }
func (c *HistoryCommand) Usage() string        { return "/history" }

func (c *HistoryCommand) Execute(args []string, cfg *config.Config) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	if len(state.Runs) == 0 {
		fmt.Println("暂无迁移记录")
		return nil
	}

	fmt.Println("┌────┬─────────────────────┬──────────┬───────┬──────┬──────┐")
	fmt.Println("│ #  │ 时间                │ 状态     │ 文件  │ 转换 │ 错误 │")
	fmt.Println("├────┼─────────────────────┼──────────┼───────┼──────┼──────┤")
	for i, run := range state.Runs {
		ts := run.StartedAt.Format("2006-01-02 15:04:05")
		fmt.Printf("│ %-2d │ %-19s │ %-8s │ %-5d │ %-4d │ %-4d │\n",
			i+1, ts, run.Status, run.Files, run.Converted, run.Errors)
	}
	fmt.Println("└────┴─────────────────────┴──────────┴───────┴──────┴──────┘")
	return nil
}
