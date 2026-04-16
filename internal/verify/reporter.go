package verify

import (
	"fmt"
	"strings"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// Reporter generates migration reports
type Reporter struct{}

func NewReporter() *Reporter {
	return &Reporter{}
}

// Generate creates a migration report from results
func (r *Reporter) Generate(results []*types.MigrationResult, verifies []*types.VerifyResult, duration time.Duration) *types.MigrationReport {
	report := &types.MigrationReport{
		TotalCases:   len(results),
		Duration:     duration,
		ByComplexity: make(map[string]*types.ComplexityStats),
	}

	for _, result := range results {
		if result == nil {
			report.Skipped++
			continue
		}

		if result.Success {
			report.Succeeded++
		} else {
			report.Failed++
			report.FailedCases = append(report.FailedCases, types.FailedCase{
				File:  result.CaseFile,
				Error: result.Error,
				Phase: "execution",
			})
		}
	}

	// Add verification failures
	for _, vr := range verifies {
		if vr == nil {
			continue
		}
		if !vr.CompileOK && vr.CompileError != "" {
			// Check if already counted as execution failure
			found := false
			for _, fc := range report.FailedCases {
				if fc.File == vr.CaseFile {
					found = true
					break
				}
			}
			if !found {
				report.FailedCases = append(report.FailedCases, types.FailedCase{
					File:  vr.CaseFile,
					Error: vr.CompileError,
					Phase: "verification",
				})
			}
		}
	}

	if report.TotalCases > 0 {
		report.SuccessRate = float64(report.Succeeded) / float64(report.TotalCases)
	}

	return report
}

// FormatText generates a text-format report for terminal output
func (r *Reporter) FormatText(report *types.MigrationReport) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("═══════════════════════════════════════════════\n")
	sb.WriteString("              CaseMover 迁移报告               \n")
	sb.WriteString("═══════════════════════════════════════════════\n")
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("  总计: %d  |  成功: %d  |  失败: %d  |  跳过: %d\n",
		report.TotalCases, report.Succeeded, report.Failed, report.Skipped))
	sb.WriteString(fmt.Sprintf("  成功率: %.1f%%\n", report.SuccessRate*100))
	sb.WriteString(fmt.Sprintf("  耗时: %s\n", report.Duration.Round(time.Second)))
	sb.WriteString("\n")

	// Complexity breakdown
	if len(report.ByComplexity) > 0 {
		sb.WriteString("  按复杂度分布:\n")
		for level, stats := range report.ByComplexity {
			sb.WriteString(fmt.Sprintf("    %s: %d/%d (%.1f%%)\n",
				level, stats.Succeeded, stats.Total, stats.Rate*100))
		}
		sb.WriteString("\n")
	}

	// Failed cases
	if len(report.FailedCases) > 0 {
		sb.WriteString("  失败清单:\n")
		sb.WriteString("  ─────────────────────────────────────────\n")
		for i, fc := range report.FailedCases {
			sb.WriteString(fmt.Sprintf("  %d. [%s] %s\n", i+1, fc.Phase, fc.File))
			if fc.Error != "" {
				// Truncate long errors
				errMsg := fc.Error
				if len(errMsg) > 200 {
					errMsg = errMsg[:200] + "..."
				}
				sb.WriteString(fmt.Sprintf("     错误: %s\n", errMsg))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("═══════════════════════════════════════════════\n")

	return sb.String()
}

// Print outputs the report to stdout
func (r *Reporter) Print(report *types.MigrationReport) {
	fmt.Print(r.FormatText(report))
}
