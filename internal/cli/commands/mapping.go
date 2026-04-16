package commands

import (
	"fmt"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// MappingCommand implements /mapping (alias /m).
// Sub-commands:
//
//	list                   - show the NepToMidscene mapping table
//	add <nep_api> <type>   - add a custom mapping entry
type MappingCommand struct{}

func NewMappingCommand() *MappingCommand { return &MappingCommand{} }

func (c *MappingCommand) Name() string        { return "mapping" }
func (c *MappingCommand) Aliases() []string    { return []string{"m"} }
func (c *MappingCommand) Description() string  { return "查看/管理 API 映射表" }
func (c *MappingCommand) Usage() string        { return "/mapping [list | add <nep_api> <type>]" }

func (c *MappingCommand) Execute(args []string, cfg *config.Config) error {
	if len(args) == 0 {
		return mappingList(cfg)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "list":
		return mappingList(cfg)
	case "add":
		return mappingAdd(args[1:], cfg)
	default:
		return fmt.Errorf("未知子命令: %s (可用: list, add)", sub)
	}
}

func mappingList(cfg *config.Config) error {
	// Built-in mapping table (from fingerprint.NepToMidsceneMapping).
	builtIn := map[string]string{
		"nep.click":       "ai.click",
		"nep.fill":        "ai.fill",
		"nep.assert":      "ai.assert",
		"nep.getText":     "ai.getText",
		"nep.waitFor":     "ai.waitFor",
		"nep.hover":       "ai.hover",
		"nep.scroll":      "ai.scroll",
		"nep.screenshot":  "ai.screenshot",
		"nep.navigate":    "page.goto",
		"nep.select":      "ai.select",
		"nep.check":       "ai.check",
		"nep.uncheck":     "ai.uncheck",
		"nep.dblclick":    "ai.dblclick",
		"nep.press":       "ai.press",
		"nep.dragAndDrop": "ai.dragAndDrop",
	}

	fmt.Println("┌──────────────────────────────────────────────────────┐")
	fmt.Println("│              API 映射表 (内置)                        │")
	fmt.Println("├───────────────────────┬──────────────────────────────┤")
	fmt.Println("│ NEP API               │ Midscene API                 │")
	fmt.Println("├───────────────────────┼──────────────────────────────┤")
	for nep, mid := range builtIn {
		fmt.Printf("│ %-21s │ %-28s │\n", nep, mid)
	}
	fmt.Println("└───────────────────────┴──────────────────────────────┘")

	// Custom mappings from config
	custom := cfg.CustomMappings
	if len(custom) > 0 {
		fmt.Println()
		fmt.Println("┌──────────────────────────────────────────────────────┐")
		fmt.Println("│              API 映射表 (自定义)                      │")
		fmt.Println("├───────────────────────┬──────────────────────────────┤")
		fmt.Println("│ NEP API               │ Midscene API                 │")
		fmt.Println("├───────────────────────┼──────────────────────────────┤")
		for nep, mid := range custom {
			fmt.Printf("│ %-21s │ %-28s │\n", nep, mid)
		}
		fmt.Println("└───────────────────────┴──────────────────────────────┘")
	}

	return nil
}

func mappingAdd(args []string, cfg *config.Config) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: /mapping add <nep_api> <midscene_api>")
	}
	nepAPI := args[0]
	midAPI := args[1]

	if cfg.CustomMappings == nil {
		cfg.CustomMappings = make(map[string]string)
	}
	cfg.CustomMappings[nepAPI] = midAPI
	fmt.Printf("已添加自定义映射: %s -> %s\n", nepAPI, midAPI)
	return nil
}
