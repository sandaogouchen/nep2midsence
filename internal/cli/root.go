package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "casemover",
	Short: "nep → midscene 自动化 Case 批量迁移工具",
	Long: `CaseMover 是一个 Go CLI 工具，通过多层次静态分析理解原始 nep case 的语义结构，
生成结构化迁移指令，然后调起本地的 Coco（Claude Code CLI）执行实际代码改写，
实现大批量 case 迁移的半自动化。`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认: .casemover.yaml)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "详细日志输出")
}
