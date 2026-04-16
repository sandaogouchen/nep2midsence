package commands

import (
	"fmt"
	"strconv"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// StartCommand implements /start (alias /s).
// Starts the migration process, reusing existing start logic.
type StartCommand struct{}

func NewStartCommand() *StartCommand { return &StartCommand{} }

func (c *StartCommand) Name() string        { return "start" }
func (c *StartCommand) Aliases() []string    { return []string{"s"} }
func (c *StartCommand) Description() string  { return "开始迁移转换" }
func (c *StartCommand) Usage() string        { return "/start [--dir <path>] [--dry-run] [-j <workers>]" }

func (c *StartCommand) Execute(args []string, cfg *config.Config) error {
	// Parse flags from args
	dir := cfg.SourceDir
	dryRun := false
	workers := cfg.Workers

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				i++
				dir = args[i]
			} else {
				return fmt.Errorf("--dir 需要一个路径参数")
			}
		case "--dry-run":
			dryRun = true
		case "-j":
			if i+1 < len(args) {
				i++
				n, err := strconv.Atoi(args[i])
				if err != nil || n < 1 {
					return fmt.Errorf("-j 需要一个正整数参数")
				}
				workers = n
			} else {
				return fmt.Errorf("-j 需要一个数字参数")
			}
		default:
			return fmt.Errorf("未知参数: %s", args[i])
		}
	}

	if dir == "" {
		dir = "."
	}

	if dryRun {
		fmt.Printf("[DRY-RUN] 模拟迁移 (目录: %s, 并发: %d)\n", dir, workers)
	} else {
		fmt.Printf("开始迁移 (目录: %s, 并发: %d)\n", dir, workers)
	}

	// Delegate to engine.
	// In a real implementation this calls:
	//   engine := migrate.NewEngine(cfg)
	//   return engine.Run(ctx, dir, dryRun, workers)
	//
	// For now we provide the wiring point.
	fmt.Println("迁移引擎启动中...")
	fmt.Println("[提示] 请实现 migrate.Engine.Run 集成此处")
	return nil
}
