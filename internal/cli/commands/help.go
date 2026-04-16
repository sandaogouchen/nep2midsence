package commands

import (
	"fmt"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// CommandInfo carries display data so the help command can list all commands
// without importing the registry (avoids circular dependency).
type CommandInfo struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
}

// HelpCommand implements /help (alias /h).
type HelpCommand struct {
	listFunc func() []CommandInfo
}

// NewHelpCommand creates a HelpCommand. listFunc may be nil and set later via SetListFunc.
func NewHelpCommand(listFunc func() []CommandInfo) *HelpCommand {
	return &HelpCommand{listFunc: listFunc}
}

// SetListFunc sets the callback used to enumerate all commands.
func (c *HelpCommand) SetListFunc(fn func() []CommandInfo) {
	c.listFunc = fn
}

func (c *HelpCommand) Name() string        { return "help" }
func (c *HelpCommand) Aliases() []string    { return []string{"h"} }
func (c *HelpCommand) Description() string  { return "显示可用命令及帮助信息" }
func (c *HelpCommand) Usage() string        { return "/help [command]" }

func (c *HelpCommand) Execute(args []string, cfg *config.Config) error {
	if c.listFunc == nil {
		fmt.Println("帮助系统尚未初始化")
		return nil
	}

	cmds := c.listFunc()

	// If a specific command name is given, show its detailed usage.
	if len(args) > 0 {
		target := strings.ToLower(args[0])
		for _, ci := range cmds {
			if strings.ToLower(ci.Name) == target || containsLower(ci.Aliases, target) {
				printCommandDetail(ci)
				return nil
			}
		}
		fmt.Printf("未知命令: %s\n", args[0])
		return nil
	}

	// Print full command table.
	fmt.Println("┌──────────────────────────────────────────────────────────┐")
	fmt.Println("│               nep2midsence 可用命令                      │")
	fmt.Println("├──────────────┬───────────┬──────────────────────────────┤")
	fmt.Println("│ 命令         │ 别名      │ 说明                         │")
	fmt.Println("├──────────────┼───────────┼──────────────────────────────┤")
	for _, ci := range cmds {
		aliases := strings.Join(ci.Aliases, ", ")
		if aliases == "" {
			aliases = "-"
		}
		fmt.Printf("│ /%-12s│ %-9s │ %-28s │\n", ci.Name, aliases, ci.Description)
	}
	fmt.Println("└──────────────┴───────────┴──────────────────────────────┘")
	fmt.Println()
	fmt.Println("提示: 输入 /help <command> 查看命令详细用法")
	return nil
}

func printCommandDetail(ci CommandInfo) {
	fmt.Println()
	fmt.Printf("命令: /%s\n", ci.Name)
	if len(ci.Aliases) > 0 {
		fmt.Printf("别名: %s\n", strings.Join(ci.Aliases, ", "))
	}
	fmt.Printf("说明: %s\n", ci.Description)
	fmt.Printf("用法: %s\n", ci.Usage)
	fmt.Println()
}

func containsLower(ss []string, target string) bool {
	for _, s := range ss {
		if strings.ToLower(s) == target {
			return true
		}
	}
	return false
}
