package commands

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// ExportCommand implements /export (alias /e).
type ExportCommand struct{}

func NewExportCommand() *ExportCommand { return &ExportCommand{} }

func (c *ExportCommand) Name() string        { return "export" }
func (c *ExportCommand) Aliases() []string    { return []string{"e"} }
func (c *ExportCommand) Description() string  { return "导出迁移报告" }
func (c *ExportCommand) Usage() string        { return "/export [json|csv|html] [--output <path>]" }

func (c *ExportCommand) Execute(args []string, cfg *config.Config) error {
	format := "json"
	if len(args) > 0 {
		format = args[0]
	}
	fmt.Printf("导出格式: %s\n", format)
	fmt.Println("[提示] 请实现 export 逻辑集成此处")
	return nil
}
