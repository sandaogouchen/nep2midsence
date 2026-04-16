package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sandaogouchen/nep2midsence/internal/analyzer"
	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/executor"
	"github.com/sandaogouchen/nep2midsence/internal/prompt"
	"github.com/sandaogouchen/nep2midsence/internal/verify"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "完整流程：分析 → 生成 → 执行 → 验证",
	Long:  "执行完整的迁移流程，包括代码分析、迁移计划生成、Coco 执行和结果验证。",
	RunE:  runStart,
}

var (
	startDir    string
	startDryRun bool
	startJobs   int
)

func init() {
	startCmd.Flags().StringVar(&startDir, "dir", "", "指定目标文件夹（不指定则交互选择）")
	startCmd.Flags().BoolVar(&startDryRun, "dry-run", false, "只生成计划，不执行")
	startCmd.Flags().IntVarP(&startJobs, "jobs", "j", 1, "并发数")

	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	// Load config
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	if startDir == "" {
		// Default to current directory if not specified
		startDir = "."
		fmt.Println("未指定目录，使用当前目录")
	}

	fmt.Printf("📂 目标目录: %s\n", startDir)

	// Phase 1: Analyze
	fmt.Println("\n══ Phase 1: 分析 ══")
	engine := analyzer.NewEngine(cfg)

	analyses, err := engine.AnalyzeDir(startDir)
	if err != nil {
		return fmt.Errorf("分析失败: %w", err)
	}

	if len(analyses) == 0 {
		fmt.Println("未找到匹配的 case 文件")
		return nil
	}

	fmt.Printf("✅ 发现 %d 个 case 文件\n", len(analyses))

	// Print complexity summary
	complexityCount := map[string]int{}
	for _, a := range analyses {
		complexityCount[a.Complexity]++
	}
	for level, count := range complexityCount {
		fmt.Printf("   %s: %d 个\n", level, count)
	}

	// Phase 2: Generate plans
	fmt.Println("\n══ Phase 2: 生成迁移计划 ══")
	gen := prompt.NewGenerator(cfg)

	for i, a := range analyses {
		promptText, err := gen.Generate(a)
		if err != nil {
			fmt.Printf("⚠️  %s: 生成 prompt 失败: %v\n", a.FilePath, err)
			continue
		}
		fmt.Printf("   [%d/%d] %s → %s (复杂度: %s, prompt: ~%d chars)\n",
			i+1, len(analyses), a.FilePath, a.TargetPath, a.Complexity, len(promptText))
	}

	if startDryRun {
		fmt.Println("\n🏁 Dry-run 模式，跳过执行和验证")
		return nil
	}

	// Phase 3: Execute
	fmt.Println("\n══ Phase 3: 执行迁移 ══")

	// Parse timeout
	timeout, _ := time.ParseDuration(cfg.Coco.Timeout)
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	cocoExec := executor.NewCocoExecutor(
		startDir,
		cfg.Coco.MaxTurns,
		cfg.Coco.AllowedTools,
		timeout,
		cfg.Coco.OutputFormat,
	)

	scheduler := executor.NewScheduler(cocoExec, gen, startJobs, cfg.Execution.RetryLimit)
	scheduler.SetProgressCallback(func(file string, success bool, current, total int) {
		status := "✅"
		if !success {
			status = "❌"
		}
		fmt.Printf("   %s [%d/%d] %s\n", status, current, total, file)
	})

	ctx := context.Background()
	start := time.Now()
	results := scheduler.Run(ctx, analyses)

	// Phase 4: Verify
	fmt.Println("\n══ Phase 4: 验证 ══")
	verifier := verify.NewVerifier(startDir, "", "")
	verifyResults := verifier.VerifyAll(results)
	_ = verifyResults

	// Generate report
	reporter := verify.NewReporter()
	report := reporter.Generate(results, verifyResults, time.Since(start))
	reporter.Print(report)

	return nil
}
