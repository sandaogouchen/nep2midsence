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

func TestGenerateCompileFixPromptMentionsCompilerErrorsAndDependencies(t *testing.T) {
	cfg := config.DefaultConfig()
	g := NewGenerator(cfg)

	promptText := g.GenerateCompileFixPrompt(
		"/tmp/case.spec.ts",
		"TypeScript compile failed: missing modules (1)\n@utils/index",
		2,
	)

	if !strings.Contains(promptText, "编译失败修复任务（第 2 次修正）") {
		t.Fatalf("prompt missing retry heading: %s", promptText)
	}
	if !strings.Contains(promptText, "TypeScript compile failed: missing modules") {
		t.Fatalf("prompt missing compile error details: %s", promptText)
	}
	if !strings.Contains(promptText, "依赖缺失时，必须迁移或替换依赖") {
		t.Fatalf("prompt missing dependency migration instruction: %s", promptText)
	}
	if !strings.Contains(promptText, "不需要运行测试") {
		t.Fatalf("prompt should explicitly skip tests: %s", promptText)
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

func TestGenerateCrossRepoPromptIncludesResolvedLocalAliasDependencies(t *testing.T) {
	cfg := config.DefaultConfig()
	dir := t.TempDir()
	cfg.Source.Dir = dir
	cfg.Target.BaseDir = filepath.Join(dir, "target-repo")
	g := NewGenerator(cfg)

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@testData/*": ["./e2e/test_data/*"],
      "@utils/*": ["./e2e/utils/*"]
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	casePath := filepath.Join(dir, "e2e", "tests", "brand", "case.spec.ts")
	if err := os.MkdirAll(filepath.Dir(casePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(case dir): %v", err)
	}
	if err := os.WriteFile(casePath, []byte(`import { CaseTags } from '@testData/index';
import { commonAfter } from '@utils/index';

test("x", async () => {
  console.log(CaseTags.P1, commonAfter);
})`), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}

	testDataIndex := filepath.Join(dir, "e2e", "test_data", "index.ts")
	if err := os.MkdirAll(filepath.Dir(testDataIndex), 0o755); err != nil {
		t.Fatalf("MkdirAll(testData dir): %v", err)
	}
	if err := os.WriteFile(testDataIndex, []byte(`export const CaseTags = { P1: "P1" };`), 0o644); err != nil {
		t.Fatalf("WriteFile(testData): %v", err)
	}

	utilsIndex := filepath.Join(dir, "e2e", "utils", "index.ts")
	if err := os.MkdirAll(filepath.Dir(utilsIndex), 0o755); err != nil {
		t.Fatalf("MkdirAll(utils dir): %v", err)
	}
	if err := os.WriteFile(utilsIndex, []byte(`export const commonAfter = async () => {};`), 0o644); err != nil {
		t.Fatalf("WriteFile(utils): %v", err)
	}

	analysis := &types.FullAnalysis{
		FilePath:   casePath,
		TargetPath: filepath.Join(cfg.Target.BaseDir, "e2e", "tests", "brand", "case.spec.ts"),
		Language:   "typescript",
		AST: &types.ASTInfo{
			Imports: []types.ImportInfo{
				{Path: "@testData/index", Name: "{ CaseTags }"},
				{Path: "@utils/index", Name: "{ commonAfter }"},
			},
		},
	}

	promptText, err := g.Generate(analysis)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !strings.Contains(promptText, "源仓库本地依赖 import") {
		t.Fatalf("prompt missing cross-repo local import dependency section: %s", promptText)
	}
	if !strings.Contains(promptText, "@testData/index") || !strings.Contains(promptText, testDataIndex) {
		t.Fatalf("prompt missing resolved @testData dependency details: %s", promptText)
	}
	if !strings.Contains(promptText, "@utils/index") || !strings.Contains(promptText, utilsIndex) {
		t.Fatalf("prompt missing resolved @utils dependency details: %s", promptText)
	}
	if !strings.Contains(promptText, "不能直接原样保留") {
		t.Fatalf("prompt missing cross-repo alias rewrite constraint: %s", promptText)
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

func TestGenerateIncludesMinimalHelperMigrationScope(t *testing.T) {
	cfg := config.DefaultConfig()
	g := NewGenerator(cfg)

	dir := t.TempDir()
	modulePath := filepath.Join(dir, "OptimizationAndBiddingModule1MNBA.ts")
	if err := os.WriteFile(modulePath, []byte(`export class OptimizationAndBiddingModule1MNBA {}`), 0o644); err != nil {
		t.Fatalf("WriteFile(module): %v", err)
	}

	analysis := &types.FullAnalysis{
		FilePath:   modulePath,
		TargetPath: filepath.Join(dir, "output", "OptimizationAndBiddingModule1MNBA.ts"),
		Language:   "typescript",
		TaskKind:   "helper",
		HelperPlan: &types.HelperMigrationPlan{
			Receiver:       "adGroupPage.optimizationAndBiddingModule1MNBA",
			PageObjectFile: filepath.Join(dir, "AdGroupPage.ts"),
			Methods:        []string{"setBid", "vv_goal_6s"},
		},
	}

	promptText, err := g.Generate(analysis)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !strings.Contains(promptText, "最小 helper 迁移范围") {
		t.Fatalf("prompt missing minimal helper migration section: %s", promptText)
	}
	if !strings.Contains(promptText, "setBid") || !strings.Contains(promptText, "vv_goal_6s") {
		t.Fatalf("prompt missing helper method list: %s", promptText)
	}
	if !strings.Contains(promptText, "adGroupPage.optimizationAndBiddingModule1MNBA") {
		t.Fatalf("prompt missing helper receiver: %s", promptText)
	}
}

func TestGenerateUsesExplicitMidsceneHelperCallsForWrapperSteps(t *testing.T) {
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
		TaskKind:   "case",
		CallChains: []*types.CallChain{{
			EntryFunc: "x",
			TestFunc:  "x",
			Steps: []types.CallStep{
				{
					Callee:        "listPage.commonActions.editAdGroup2",
					FullReceiver:  "listPage.commonActions",
					FuncName:      "editAdGroup2",
					Args:          []string{"adgroupName", "campaignId"},
					OwnerKind:     "business",
					IsWrapperCall: true,
				},
				{
					Callee:        "adGroupPage.optimizationAndBiddingModule1MNBA.setBid",
					FullReceiver:  "adGroupPage.optimizationAndBiddingModule1MNBA",
					FuncName:      "setBid",
					Args:          []string{"\"2\""},
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

	for _, want := range []string{
		"await listPage.commonActions.editAdGroup2Midscene(agent, adgroupName, campaignId)",
		"await adGroupPage.optimizationAndBiddingModule1MNBA.setBidMidscene(agent, \"2\")",
	} {
		if !strings.Contains(promptText, want) {
			t.Fatalf("prompt missing explicit wrapper target call %q: %s", want, promptText)
		}
	}
}

func TestGenerateIncludesUnresolvedHelperTodoInstructions(t *testing.T) {
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
		TaskKind:   "case",
		UnresolvedHelpers: []types.UnresolvedHelper{
			{
				Receiver: "adGroupPage.optimizationAndBiddingModule1MNBA",
				Method:   "setBid",
				Reason:   "method definition not found in module source",
			},
		},
	}

	promptText, err := g.Generate(analysis)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !strings.Contains(promptText, "未解析 helper 依赖") {
		t.Fatalf("prompt missing unresolved helper section: %s", promptText)
	}
	if !strings.Contains(promptText, "TODO(nep2midsence)") {
		t.Fatalf("prompt missing TODO instruction for unresolved helper: %s", promptText)
	}
	if !strings.Contains(promptText, "setBid") {
		t.Fatalf("prompt missing unresolved helper method name: %s", promptText)
	}
}
