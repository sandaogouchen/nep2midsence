package commands

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// ClearCommand implements /clear (alias /cl).
type ClearCommand struct{}

func NewClearCommand() *ClearCommand { return &ClearCommand{} }

func (c *ClearCommand) Name() string        { return "clear" }
func (c *ClearCommand) Aliases() []string    { return []string{"cl"} }
func (c *ClearCommand) Description() string  { return "清空屏幕" }
func (c *ClearCommand) Usage() string        { return "/clear" }

func (c *ClearCommand) Execute(args []string, cfg *config.Config) error {
	// ANSI escape to clear screen and move cursor to top-left.
	fmt.Print("\033[2J\033[H")
	return nil
}
