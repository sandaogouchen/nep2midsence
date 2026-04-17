package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

func TestMatchesPatternFallsBackToExtensionsWhenFilePatternsEmpty(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	if !engine.matchesPattern("brand_case.ts") {
		t.Fatal("matchesPattern returned false for .ts file with default config")
	}
}

func TestMatchesPatternChecksFilePatternsAndExtensionsIndependently(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Source.FilePatterns = []string{"*_test.go"}
	// Default Extensions includes ".ts", so .ts files should still match
	// even though FilePatterns only lists Go test globs.
	engine := NewEngine(cfg)

	if !engine.matchesPattern("brand_case.ts") {
		t.Fatal("matchesPattern returned false for .ts file; Extensions should match independently of FilePatterns")
	}
	if !engine.matchesPattern("brand_case_test.go") {
		t.Fatal("matchesPattern returned false for matching explicit file_patterns entry")
	}
	if engine.matchesPattern("brand_case.py") {
		t.Fatal("matchesPattern returned true for .py file which is in neither FilePatterns nor Extensions")
	}
}

func TestAnalyzeFileSupportsTypeScriptInputs(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "sample.spec.ts")
	source := `import { test } from "@playwright/test"

test("edits campaign", async ({ page }) => {
  await page.click("#login-btn")
})
`
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write ts file: %v", err)
	}

	result, err := engine.AnalyzeFile(filePath)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error for ts input: %v", err)
	}
	if result == nil {
		t.Fatal("AnalyzeFile returned nil result for ts input")
	}
	if result.AST == nil {
		t.Fatal("AnalyzeFile returned nil AST for ts input")
	}
	if len(result.CallChains) == 0 {
		t.Fatal("AnalyzeFile returned no call chains for ts input")
	}
}

func TestAnalyzeFileCapturesMultiLevelReceiverChain(t *testing.T) {
	cfg := config.DefaultConfig()
	// Default config points TSExtractor to scripts/dist/ts-ast-extractor.js. In unit tests
	// this file typically doesn't exist, so the engine falls back to regex extraction.
	engine := NewEngine(cfg)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "sample.spec.ts")
	source := `test("wrapper calls", async () => {
  await adGroupPage.optimizationAndBiddingModule1MNBA.vv_setStandardBtn()
  await adGroupPage.optimizationAndBiddingModule1MNBA.vv_goal_6s()
})
`
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write ts file: %v", err)
	}

	result, err := engine.AnalyzeFile(filePath)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error for ts input: %v", err)
	}
	if result == nil || len(result.CallChains) == 0 {
		t.Fatalf("AnalyzeFile returned empty result for ts input")
	}

	found := false
	for _, ch := range result.CallChains {
		for _, st := range ch.Steps {
			if strings.Contains(st.Callee, "vv_setStandardBtn") {
				found = true
				if st.FullReceiver != "adGroupPage.optimizationAndBiddingModule1MNBA" {
					t.Fatalf("FullReceiver = %q, want %q", st.FullReceiver, "adGroupPage.optimizationAndBiddingModule1MNBA")
				}
				if !st.IsWrapperCall {
					t.Fatalf("IsWrapperCall = false, want true")
				}
				if st.Callee != "adGroupPage.optimizationAndBiddingModule1MNBA.vv_setStandardBtn" {
					t.Fatalf("Callee = %q, want full callee", st.Callee)
				}
			}
		}
	}
	if !found {
		t.Fatalf("did not find wrapper call in extracted steps")
	}
}

func TestExtractDefaultPrompts(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "VVGoalBtn.ts")
	source := "export class VVGoalBtn {\n" +
		"  static DEFAULT_PROMPT: string = `[goal] 文案下方的下拉icon`;\n" +
		"}\n"
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write ts file: %v", err)
	}

	ps := ExtractDefaultPrompts(filePath)
	if len(ps) != 1 {
		t.Fatalf("ExtractDefaultPrompts returned %d entries, want 1", len(ps))
	}
	if ps[0].ClassName != "VVGoalBtn" {
		t.Fatalf("ClassName = %q, want VVGoalBtn", ps[0].ClassName)
	}
	if ps[0].PromptValue != "[goal] 文案下方的下拉icon" {
		t.Fatalf("PromptValue = %q", ps[0].PromptValue)
	}
}
