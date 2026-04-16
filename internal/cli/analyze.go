package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sandaogouchen/nep2midsence/internal/analyzer"
	"github.com/sandaogouchen/nep2midsence/internal/config"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "仅执行分析阶段",
	Long:  "对目标目录执行 5 层分析，输出分析结果（JSON 格式）。",
	RunE:  runAnalyze,
}

var analyzeDir string
var analyzeOutput string

func init() {
	analyzeCmd.Flags().StringVar(&analyzeDir, "dir", ".", "目标文件夹")
	analyzeCmd.Flags().StringVarP(&analyzeOutput, "output", "o", "", "输出文件路径（默认输出到 stdout）")

	rootCmd.AddCommand(analyzeCmd)
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	engine := analyzer.NewEngine(cfg)
	analyses, err := engine.AnalyzeDir(analyzeDir)
	if err != nil {
		return fmt.Errorf("分析失败: %w", err)
	}

	data, err := json.MarshalIndent(analyses, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	if analyzeOutput != "" {
		if err := os.WriteFile(analyzeOutput, data, 0644); err != nil {
			return fmt.Errorf("写入文件失败: %w", err)
		}
		fmt.Printf("分析结果已写入: %s\n", analyzeOutput)
	} else {
		fmt.Println(string(data))
	}

	return nil
}
