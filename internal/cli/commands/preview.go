package commands

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// PreviewCommand implements /preview (alias /p).
type PreviewCommand struct{}

func NewPreviewCommand() *PreviewCommand { return &PreviewCommand{} }

func (c *PreviewCommand) Name() string        { return "preview" }
func (c *PreviewCommand) Aliases() []string    { return []string{"p"} }
func (c *PreviewCommand) Description() string  { return "预览迁移结果" }
func (c *PreviewCommand) Usage() string        { return "/preview [file]" }

func (c *PreviewCommand) Execute(args []string, cfg *config.Config) error {
	if len(args) == 0 {
		fmt.Println("用法: /preview <file>")
		return nil
	}
	fmt.Printf("预览文件: %s\n", args[0])
	fmt.Println("[提示] 请实现 preview 逻辑集成此处")
	return nil
}
