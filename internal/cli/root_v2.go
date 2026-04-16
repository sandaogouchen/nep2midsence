package cli

import (
	"fmt"

	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root cobra command.
// When invoked without a subcommand, it launches the interactive REPL.
//
// This file demonstrates how the existing root.go should be modified
// to integrate the REPL. Merge the RunE function below into your
// existing rootCmd definition.
func NewRootCmd() *cobra.Command {
	var cfgFile string

	rootCmd := &cobra.Command{
		Use:   "nep2midsence",
		Short: "nep 到 midscene 测试迁移工具",
		Long: `nep2midsence v2.0.0

将 nep 端到端测试自动迁移为 midscene 格式。
直接运行（无子命令）进入交互式 REPL。`,

		// SilenceUsage prevents cobra from printing usage on errors from RunE.
		SilenceUsage: true,

		// RunE is invoked when no subcommand is given.
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. Load configuration
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("加载配置失败: %w", err)
			}

			// 2. Enter REPL
			return StartREPL(cfg)
		},
	}

	// Persistent flags available to all subcommands.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认 .nep2midsence.yaml)")

	// Register existing subcommands here:
	//   rootCmd.AddCommand(newStartCmd())
	//   rootCmd.AddCommand(newAnalyzeCmd())
	//   rootCmd.AddCommand(newVersionCmd())
	//   ... etc.

	return rootCmd
}

// Execute runs the root command. Call this from main().
func Execute() error {
	return NewRootCmd().Execute()
}
