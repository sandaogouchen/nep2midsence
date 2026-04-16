package commands

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// RollbackCommand implements /rollback (alias /rb).
type RollbackCommand struct{}

func NewRollbackCommand() *RollbackCommand { return &RollbackCommand{} }

func (c *RollbackCommand) Name() string        { return "rollback" }
func (c *RollbackCommand) Aliases() []string    { return []string{"rb"} }
func (c *RollbackCommand) Description() string  { return "回滚上次迁移" }
func (c *RollbackCommand) Usage() string        { return "/rollback [--all | --file <path>]" }

func (c *RollbackCommand) Execute(args []string, cfg *config.Config) error {
	fmt.Println("正在回滚...")
	fmt.Println("[提示] 请实现 rollback 逻辑集成此处")
	return nil
}
