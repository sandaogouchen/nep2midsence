package commands

import (
	"fmt"
	"os"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// QuitCommand implements /quit (alias /q).
type QuitCommand struct{}

func NewQuitCommand() *QuitCommand { return &QuitCommand{} }

func (c *QuitCommand) Name() string        { return "quit" }
func (c *QuitCommand) Aliases() []string    { return []string{"q", "exit"} }
func (c *QuitCommand) Description() string  { return "退出 REPL" }
func (c *QuitCommand) Usage() string        { return "/quit" }

func (c *QuitCommand) Execute(args []string, cfg *config.Config) error {
	fmt.Println("再见!")
	os.Exit(0)
	return nil // unreachable
}
