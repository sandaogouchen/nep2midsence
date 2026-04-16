package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// ConfigCommand implements /config (alias /c).
// Sub-commands:
//
//	(no args)        - show all config values
//	set <key> <val>  - set a config value
//	load <file>      - load config from file
//	save             - save current config to file
//	reset            - reset config to defaults
type ConfigCommand struct{}

func NewConfigCommand() *ConfigCommand { return &ConfigCommand{} }

func (c *ConfigCommand) Name() string       { return "config" }
func (c *ConfigCommand) Aliases() []string   { return []string{"c"} }
func (c *ConfigCommand) Description() string { return "查看/修改配置" }
func (c *ConfigCommand) Usage() string {
	return "/config [set <key> <value> | load <file> | save | reset]"
}

func (c *ConfigCommand) Execute(args []string, cfg *config.Config) error {
	if len(args) == 0 {
		return showConfig(cfg)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "set":
		return configSet(args[1:], cfg)
	case "load":
		return configLoad(args[1:], cfg)
	case "save":
		return configSave(cfg)
	case "reset":
		return configReset(cfg)
	default:
		return fmt.Errorf("未知子命令: %s (可用: set, load, save, reset)", sub)
	}
}

// showConfig prints all config key-value pairs in a table.
func showConfig(cfg *config.Config) error {
	kvs := cfg.ToMap()
	// Sort keys for stable output.
	keys := make([]string, 0, len(kvs))
	for k := range kvs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Println("┌───────────────────────────────────────────────────────┐")
	fmt.Println("│                   当前配置                            │")
	fmt.Println("├──────────────────────┬────────────────────────────────┤")
	fmt.Println("│ 键                   │ 值                             │")
	fmt.Println("├──────────────────────┼────────────────────────────────┤")
	for _, k := range keys {
		v := fmt.Sprintf("%v", kvs[k])
		if len(v) > 30 {
			v = v[:27] + "..."
		}
		fmt.Printf("│ %-20s │ %-30s │\n", k, v)
	}
	fmt.Println("└──────────────────────┴────────────────────────────────┘")
	return nil
}

func configSet(args []string, cfg *config.Config) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: /config set <key> <value>")
	}
	key := args[0]
	value := strings.Join(args[1:], " ")

	if err := cfg.Set(key, value); err != nil {
		return fmt.Errorf("设置失败: %w", err)
	}
	fmt.Printf("已设置 %s = %s\n", key, value)
	return nil
}

func configLoad(args []string, cfg *config.Config) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: /config load <file>")
	}
	path := args[0]
	if err := cfg.LoadFile(path); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	fmt.Printf("已从 %s 加载配置\n", path)
	return nil
}

func configSave(cfg *config.Config) error {
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	fmt.Println("配置已保存")
	return nil
}

func configReset(cfg *config.Config) error {
	cfg.Reset()
	fmt.Println("配置已重置为默认值")
	return nil
}
