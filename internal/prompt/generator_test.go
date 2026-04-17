package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandaogouchen/nep2midsence/internal/analyzer"
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
	if !strings.Contains(promptText, casePath) {
		t.Fatalf("prompt missing source file path: %s", promptText)
	}
	if !strings.Contains(promptText, analysis.TargetPath) {
		t.Fatalf("prompt missing target file path: %s", promptText)
	}
	if !strings.Contains(promptText, helperPath) {
		t.Fatalf("prompt missing related helper file path: %s", promptText)
	}
	if strings.Contains(promptText, "async function fillForm(page)") {
		t.Fatalf("prompt should not inline source code (fillForm)")
	}
	if strings.Contains(promptText, "export async function openBrandPage(page)") {
		t.Fatalf("prompt should not inline related helper code (openBrandPage)")
	}
}

func TestGenerateSkipsInfrastructureOwnerSteps(t *testing.T) {
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
			Steps: []types.CallStep{
				{
					Callee:        "page.sessionStorage.set",
					FullReceiver:  "page.sessionStorage",
					FuncName:      "set",
					OwnerKind:     "infrastructure",
					IsWrapperCall: true,
				},
				{
					Callee:        "listPage.commonActions.editCampaign2",
					FullReceiver:  "listPage.commonActions",
					FuncName:      "editCampaign2",
					OwnerKind:     "business",
					IsWrapperCall: true,
				},
			},
		}},
	}

	promptText, err := g.Generate(analysis)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if strings.Contains(promptText, "page.sessionStorage.set") {
		t.Fatalf("prompt should skip infrastructure owner step: %s", promptText)
	}
	if !strings.Contains(promptText, "listPage.commonActions.editCampaign2") {
		t.Fatalf("prompt should keep business wrapper step: %s", promptText)
	}
}

func TestGenerateSkipsForceInfraMethodSteps(t *testing.T) {
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
			Steps: []types.CallStep{
				{
					Callee:        "waitForPageLoadStable",
					FuncName:      "waitForPageLoadStable",
					OwnerKind:     "unknown",
					IsWrapperCall: true,
				},
				{
					Callee:        "listPage.commonActions.editCampaign2",
					FullReceiver:  "listPage.commonActions",
					FuncName:      "editCampaign2",
					OwnerKind:     "business",
					IsWrapperCall: true,
				},
			},
		}},
	}

	promptText, err := g.Generate(analysis)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if strings.Contains(promptText, "[封装方法]: waitForPageLoadStable()") {
		t.Fatalf("prompt should skip configured infrastructure method: %s", promptText)
	}
	if !strings.Contains(promptText, "listPage.commonActions.editCampaign2") {
		t.Fatalf("prompt should keep business wrapper step: %s", promptText)
	}
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
				OwnerKind:     "business",
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

func TestGenerateIncludesBusinessWrapperStepsInsideCommonIt(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := analyzer.NewEngine(cfg)
	g := NewGenerator(cfg)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.spec.ts")
	caseSource := `import { commonIt } from "@utils/nep_utils/CommonIt/CommonIt"

	describe('vv_cbo_afterview', () => {
	  const caseName = 'vv_cbo_afterview'
	  it(
	    caseName,
	    { timeout: 60 * 1000 },
	    commonIt('mock-url', async ({ page, campaignPage, adGroupPage }) => {
	      const campaignName = caseName + Date.now();
	      await campaignPage.campaignNameModule.setCampaignName(campaignName);
	      await campaignPage.campaignBudgetRadio.click();
	      await campaignPage.campaignNameModule1MN.setCBOBudgetStrategy(BudgetMode.Daily, '20');
	      await campaignPage.continueBtn.click();
	      const adGroupName = 'Ad group' + Date.now();
	      await adGroupPage.adGroupNameInput.type(adGroupName);
	      await adGroupPage.optimizationAndBiddingModule1MNBA.vv_setAfterViewBtn();
	      await adGroupPage.continueBtn.click();
	    }),
	  );
	})
	`
	if err := os.WriteFile(casePath, []byte(caseSource), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}

	analysis, err := engine.AnalyzeFile(casePath)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	promptText, err := g.Generate(analysis)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	for _, want := range []string{
		"campaignPage.campaignNameModule.setCampaignName",
		"campaignPage.campaignNameModule1MN.setCBOBudgetStrategy",
		"adGroupPage.optimizationAndBiddingModule1MNBA.vv_setAfterViewBtn",
	} {
		if !strings.Contains(promptText, want) {
			t.Fatalf("prompt missing wrapper step %q: %s", want, promptText)
		}
	}
}
