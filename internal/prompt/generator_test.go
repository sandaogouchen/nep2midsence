package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/types"
)

func TestNewGeneratorLoadsDefaultMigrationKnowledge(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MigrationDoc = ""

	g := NewGenerator(cfg)

	if strings.TrimSpace(g.migrationDoc) == "" {
		t.Fatal("migrationDoc is empty, want embedded default migration knowledge")
	}
	if !strings.Contains(g.migrationDoc, "NEP → Midscene") {
		t.Fatalf("migrationDoc = %q, want embedded migration knowledge heading", g.migrationDoc)
	}
}

func TestGenerateUsesPathOnlyMode(t *testing.T) {
	cfg := config.DefaultConfig()
	g := NewGenerator(cfg)

	dir := t.TempDir()
	helperDir := filepath.Join(dir, "helpers")
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	casePath := filepath.Join(dir, "brand.spec.ts")
	helperPath := filepath.Join(helperDir, "page.ts")
	caseSource := `import { openBrandPage } from "./helpers/page"

test("edit campaign", async ({ page }) => {
  await openBrandPage(page)
  await fillForm(page)
})

async function fillForm(page) {
  await page.click("#save")
}

func TestGenerateIncludesWrapperStepsAndDefaultPrompts(t *testing.T) {
	cfg := config.DefaultConfig()
	g := NewGenerator(cfg)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.spec.ts")
	if err := os.WriteFile(casePath, []byte("test(\"x\", async () => {})"), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}

	analysis := &types.FullAnalysis{
		FilePath:   casePath,
		TargetPath: filepath.Join(dir, "output", "case.spec.ts"),
		Language:   "typescript",
		CallChains: []*types.CallChain{{
			EntryFunc: "x",
			TestFunc:  "x",
			Steps: []types.CallStep{{
				Callee:        "adGroupPage.optimizationAndBiddingModule1MNBA.vv_setStandardBtn",
				FullReceiver:  "adGroupPage.optimizationAndBiddingModule1MNBA",
				Receiver:      "optimizationAndBiddingModule1MNBA",
				FuncName:      "vv_setStandardBtn",
				IsWrapperCall: true,
			}},
		}},
		DefaultPrompts: []types.DefaultPromptInfo{{
			ClassName:   "VVGoalBtn",
			PromptValue: "[goal] 文案下方的下拉icon",
			FilePath:    filepath.Join(dir, "VVGoalBtn.ts"),
			Line:        2,
		}},
	}

	promptText, err := g.Generate(analysis)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !strings.Contains(promptText, "[封装方法]") {
		t.Fatalf("prompt missing wrapper step type marker: %s", promptText)
	}
	if !strings.Contains(promptText, "组件 DEFAULT_PROMPT 映射") {
		t.Fatalf("prompt missing DEFAULT_PROMPT section")
	}
	if !strings.Contains(promptText, "[goal] 文案下方的下拉icon") {
		t.Fatalf("prompt missing DEFAULT_PROMPT value")
	}
}
`
	helperSource := `export async function openBrandPage(page) {
  await page.goto("/brand")
}
`
	if err := os.WriteFile(casePath, []byte(caseSource), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}
	if err := os.WriteFile(helperPath, []byte(helperSource), 0o644); err != nil {
		t.Fatalf("WriteFile(helper): %v", err)
	}

	analysis := &types.FullAnalysis{
		FilePath:   casePath,
		TargetPath: filepath.Join(dir, "output", "brand.spec.ts"),
		Language:   "typescript",
		Dependencies: []string{
			helperPath,
		},
		AST: &types.ASTInfo{
			Imports: []types.ImportInfo{{Path: "./helpers/page"}},
			Functions: []types.FuncInfo{
				{Name: "edit campaign", IsTest: true, LineStart: 3, LineEnd: 6},
				{Name: "fillForm", IsHelper: true, LineStart: 8, LineEnd: 10},
			},
		},
		CallChains: []*types.CallChain{{
			EntryFunc:   "edit campaign",
			TestFunc:    "edit campaign",
			Steps:       []types.CallStep{{Caller: "fillForm", InFunc: "fillForm"}},
			NepAPICalls: []types.CallStep{{Caller: "fillForm", InFunc: "fillForm", Callee: "page.click"}},
		}},
	}

	promptText, err := g.Generate(analysis)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	// Path-only: should provide file paths instead of inlining code.
	if !strings.Contains(promptText, casePath) {
		t.Fatalf("prompt missing source file path: %s", promptText)
	}
	if !strings.Contains(promptText, analysis.TargetPath) {
		t.Fatalf("prompt missing target file path: %s", promptText)
	}
	if !strings.Contains(promptText, helperPath) {
		t.Fatalf("prompt missing related helper file path: %s", promptText)
	}
	// if !strings.Contains(promptText, filepath.Join("internal", "prompt", "assets", "midsence_agent.md")) {
	// 	t.Fatalf("prompt missing midsence agent doc path: %s", promptText)
	// }
	if strings.Contains(promptText, "async function fillForm(page)") {
		t.Fatalf("prompt should not inline source code (fillForm)")
	}
	if strings.Contains(promptText, "export async function openBrandPage(page)") {
		t.Fatalf("prompt should not inline related helper code (openBrandPage)")
	}
}
