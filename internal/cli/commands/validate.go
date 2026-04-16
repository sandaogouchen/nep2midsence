package commands

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// ValidateCommand implements /validate (alias /val).
type ValidateCommand struct{}

func NewValidateCommand() *ValidateCommand { return &ValidateCommand{} }

func (c *ValidateCommand) Name() string        { return "validate" }
func (c *ValidateCommand) Aliases() []string    { return []string{"val"} }
func (c *ValidateCommand) Description() string  { return "验证迁移后的文件" }
func (c *ValidateCommand) Usage() string        { return "/validate [--dir <path>]" }

func (c *ValidateCommand) Execute(args []string, cfg *config.Config) error {
	dir := cfg.SourceDir
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			i++
			dir = args[i]
		}
	}
	fmt.Printf("验证目录: %s\n", dir)
	fmt.Println("[提示] 请实现 validate 逻辑集成此处")
	return nil
}
