package commands

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// AnalyzeCommand implements /analyze (alias /a).
// Scans the source directory and reports migration statistics without converting.
type AnalyzeCommand struct{}

func NewAnalyzeCommand() *AnalyzeCommand { return &AnalyzeCommand{} }

func (c *AnalyzeCommand) Name() string        { return "analyze" }
func (c *AnalyzeCommand) Aliases() []string    { return []string{"a"} }
func (c *AnalyzeCommand) Description() string  { return "分析源目录的 nep 测试文件" }
func (c *AnalyzeCommand) Usage() string        { return "/analyze [--dir <path>]" }

func (c *AnalyzeCommand) Execute(args []string, cfg *config.Config) error {
	dir := cfg.SourceDir
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				i++
				dir = args[i]
			} else {
				return fmt.Errorf("--dir 需要一个路径参数")
			}
		default:
			return fmt.Errorf("未知参数: %s", args[i])
		}
	}

	if dir == "" {
		dir = "."
	}

	fmt.Printf("正在分析目录: %s\n", dir)
	fmt.Println()

	// Delegate to analyzer.
	// In a real implementation this calls:
	//   analyzer := analyze.NewAnalyzer(cfg)
	//   report, err := analyzer.Scan(dir)
	//   printReport(report)
	//
	// Placeholder output:
	fmt.Println("┌─────────────────────────────────────┐")
	fmt.Println("│          分析结果摘要                │")
	fmt.Println("├─────────────────────────────────────┤")
	fmt.Println("│ 扫描文件数:        (待集成)         │")
	fmt.Println("│ 可迁移文件:        (待集成)         │")
	fmt.Println("│ 需手动处理:        (待集成)         │")
	fmt.Println("│ API 调用统计:      (待集成)         │")
	fmt.Println("└─────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("[提示] 请实现 analyze.Analyzer.Scan 集成此处")
	return nil
}
