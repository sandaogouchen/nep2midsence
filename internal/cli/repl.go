package cli

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/chzyer/readline"
	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// StartREPL launches the interactive REPL mode.
func StartREPL(cfg *config.Config) error {
	// 1. Create command registry
	registry := NewCommandRegistry(cfg)

	// 2. Build completer from registry
	completer := buildCompleter(registry)

	// 3. Create readline instance
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "nep2midsence> ",
		HistoryFile:     ".nep2midsence_history",
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return fmt.Errorf("初始化 readline 失败: %w", err)
	}
	defer rl.Close()

	// 4. Print welcome banner
	fmt.Println("==========================================================")
	fmt.Println("  nep2midsence v2.0.0 - nep 到 midscene 测试迁移工具")
	fmt.Println("==========================================================")
	fmt.Println("  输入 /help 查看可用命令，Tab 补全")
	fmt.Println()

	// 5. Main REPL loop
	for {
		line, err := rl.Readline()
		if err != nil { // io.EOF or interrupt
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Commands start with /
		if !strings.HasPrefix(line, "/") {
			fmt.Println("提示: 命令以 / 开头，输入 /help 查看帮助")
			continue
		}

		parts := splitArgs(line[1:]) // strip leading /
		if len(parts) == 0 {
			continue
		}

		cmdName := parts[0]
		args := parts[1:]

		cmd := registry.Find(cmdName)
		if cmd == nil {
			fmt.Printf("未知命令: /%s，输入 /help 查看帮助\n", cmdName)
			continue
		}

		if err := cmd.Execute(args, cfg); err != nil {
			fmt.Printf("错误: %v\n", err)
		}
	}

	fmt.Println("\n再见!")
	return nil
}

// splitArgs splits input respecting quoted strings.
// e.g. `config set key "hello world"` -> ["config", "set", "key", "hello world"]
func splitArgs(input string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range input {
		switch {
		case inQuote:
			if r == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = true
			quoteChar = r
		case unicode.IsSpace(r):
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// buildCompleter creates a readline PrefixCompleter from every registered command.
func buildCompleter(registry *CommandRegistry) *readline.PrefixCompleter {
	items := make([]readline.PrefixCompleterInterface, 0, len(registry.All())*2)
	for _, cmd := range registry.All() {
		// Primary name
		items = append(items, readline.PcItem("/"+cmd.Name(), subCompleters(cmd)...))
		// Aliases
		for _, alias := range cmd.Aliases() {
			items = append(items, readline.PcItem("/"+alias, subCompleters(cmd)...))
		}
	}
	return readline.NewPrefixCompleter(items...)
}

// subCompleters returns sub-completions for commands that have sub-commands.
func subCompleters(cmd REPLCommand) []readline.PrefixCompleterInterface {
	switch cmd.Name() {
	case "config":
		return []readline.PrefixCompleterInterface{
			readline.PcItem("set"),
			readline.PcItem("load"),
			readline.PcItem("save"),
			readline.PcItem("reset"),
		}
	case "mapping":
		return []readline.PrefixCompleterInterface{
			readline.PcItem("list"),
			readline.PcItem("add"),
		}
	case "start":
		return []readline.PrefixCompleterInterface{
			readline.PcItem("--dir"),
			readline.PcItem("--dry-run"),
			readline.PcItem("-j"),
		}
	case "export":
		return []readline.PrefixCompleterInterface{
			readline.PcItem("json"),
			readline.PcItem("csv"),
			readline.PcItem("html"),
		}
	default:
		return nil
	}
}
