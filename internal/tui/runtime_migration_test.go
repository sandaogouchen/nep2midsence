package tui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/analyzer"
	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/executor"
	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// --- isMidsceneText ---

func TestIsMidsceneTextDetectsAgentAPIs(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"aiTap", `await agent.aiTap("点击按钮");`, true},
		{"aiInput", `await agent.aiInput("value", "输入框");`, true},
		{"aiAssert", `await agent.aiAssert("展示元素");`, true},
		{"aiWaitFor", `await agent.aiWaitFor("展示目标");`, true},
		{"aiHover", `await agent.aiHover("元素");`, true},
		{"aiScroll", `agent.aiScroll('App', {direction: "down"});`, true},
		{"aiAction", `await agent.aiAction("操作");`, true},
		{"aiAct", `await agent.aiAct("操作");`, true},
		{"midsceneNewPage", `page := midscene.NewPage(browser)`, true},
		{"no_midscene_markers", `await page.click('.btn');`, false},
		{"nep_only", `await ai?.action('点击按钮');`, false},
		{"empty", ``, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMidsceneText(tt.text)
			if got != tt.want {
				t.Errorf("isMidsceneText(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

// --- isHelperAlreadyMigrated ---

func TestIsHelperAlreadyMigratedTargetNotExist(t *testing.T) {
	got := isHelperAlreadyMigrated("/nonexistent/path/file.ts", []string{"myFunc"})
	if got {
		t.Error("expected false for nonexistent target path")
	}
}

func TestIsHelperAlreadyMigratedEmptyPath(t *testing.T) {
	got := isHelperAlreadyMigrated("", []string{"myFunc"})
	if got {
		t.Error("expected false for empty target path")
	}
}

func TestIsHelperAlreadyMigratedFullyMigrated(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "helper.ts")
	content := `
import { agent } from 'midscene';

export async function clickSubmitButton() {
    await agent.aiTap("点击Submit按钮");
}

export async function fillUsername(name: string) {
    await agent.aiInput(name, "用户名输入框");
}
`
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Has midscene markers, no NEP markers, contains function names
	got := isHelperAlreadyMigrated(target, []string{"clickSubmitButton", "fillUsername"})
	if !got {
		t.Error("expected true for fully migrated helper with matching functions")
	}
}

func TestIsHelperAlreadyMigratedPartiallyMigrated(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "helper.ts")
	// Contains BOTH midscene and NEP markers — partially migrated
	content := `
import { agent } from 'midscene';

export async function clickSubmitButton() {
    await agent.aiTap("点击Submit按钮");
}

export async function legacyAction() {
    await ai?.action('旧操作');
}
`
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := isHelperAlreadyMigrated(target, []string{"clickSubmitButton"})
	if got {
		t.Error("expected false for partially migrated helper (still has NEP calls)")
	}
}

func TestIsHelperAlreadyMigratedNoMidsceneMarkers(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "helper.ts")
	// Plain file with no framework markers
	content := `
export function clickSubmitButton() {
    document.querySelector('.submit').click();
}
`
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := isHelperAlreadyMigrated(target, []string{"clickSubmitButton"})
	if got {
		t.Error("expected false for file without midscene markers")
	}
}

func TestIsHelperAlreadyMigratedFunctionNotFound(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "helper.ts")
	// Has midscene markers, no NEP, but doesn't contain the requested function
	content := `
import { agent } from 'midscene';

export async function otherFunc() {
    await agent.aiTap("其他操作");
}
`
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := isHelperAlreadyMigrated(target, []string{"clickSubmitButton"})
	if got {
		t.Error("expected false when expected function not found in target")
	}
}

func TestIsHelperAlreadyMigratedEmptyFuncNamesFrameworkOnly(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "helper.ts")
	content := `
import { agent } from 'midscene';

export async function someAction() {
    await agent.aiTap("操作");
}
`
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// No function names to check — pure framework-level check
	got := isHelperAlreadyMigrated(target, nil)
	if !got {
		t.Error("expected true when no func names required and midscene markers present")
	}
}

func TestIsHelperAlreadyMigratedMatchesFunctionColon(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "helper.ts")
	// TS object method pattern: myMethod: async () => {}
	content := `
import { agent } from 'midscene';

const actions = {
    clickSubmitButton: async () => {
        await agent.aiTap("提交按钮");
    }
};
`
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := isHelperAlreadyMigrated(target, []string{"clickSubmitButton"})
	if !got {
		t.Error("expected true for colon-style function definition matching")
	}
}

// --- extractDefinedFuncNames ---

func TestExtractDefinedFuncNamesNilAnalysis(t *testing.T) {
	got := extractDefinedFuncNames(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestExtractDefinedFuncNamesNilAST(t *testing.T) {
	got := extractDefinedFuncNames(&types.FullAnalysis{})
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestExtractDefinedFuncNamesFiltersTestFunctions(t *testing.T) {
	a := &types.FullAnalysis{
		AST: &types.ASTInfo{
			Functions: []types.FuncInfo{
				{Name: "clickButton", IsTest: false},
				{Name: "TestLogin", IsTest: true},
				{Name: "fillForm", IsTest: false},
				{Name: "", IsTest: false},
			},
		},
	}

	got := extractDefinedFuncNames(a)
	want := []string{"clickButton", "fillForm"}

	if len(got) != len(want) {
		t.Fatalf("extractDefinedFuncNames returned %d names, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTracePropertyChainDepsFindsPageObjectAndModule(t *testing.T) {
	dir := t.TempDir()

	// Simulate a typical e2e tree:
	// e2e/tests/.../case.spec.ts
	// e2e/pages/new_pages/adGroupPage/AdGroupPage.ts
	// e2e/pages/new_pages/adGroupPage/module/OptimizationAndBidding/OptimizationAndBiddingModule1MNBA.ts
	e2eDir := filepath.Join(dir, "e2e")
	caseDir := filepath.Join(e2eDir, "tests", "new_tests", "brand", "video-view")
	poDir := filepath.Join(e2eDir, "pages", "new_pages", "adGroupPage")
	modDir := filepath.Join(poDir, "module", "OptimizationAndBidding")

	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(caseDir): %v", err)
	}
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modDir): %v", err)
	}

	casePath := filepath.Join(caseDir, "case.spec.ts")
	if err := os.WriteFile(casePath, []byte(`test("x", async () => {})`), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}

	poPath := filepath.Join(poDir, "AdGroupPage.ts")
	poSource := `import { OptimizationAndBiddingModule1MNBA } from "./module/OptimizationAndBidding/OptimizationAndBiddingModule1MNBA";

export class AdGroupPage {
  public optimizationAndBiddingModule1MNBA: OptimizationAndBiddingModule1MNBA;
}
`
	if err := os.WriteFile(poPath, []byte(poSource), 0o644); err != nil {
		t.Fatalf("WriteFile(po): %v", err)
	}

	modPath := filepath.Join(modDir, "OptimizationAndBiddingModule1MNBA.ts")
	if err := os.WriteFile(modPath, []byte(`export class OptimizationAndBiddingModule1MNBA {}`), 0o644); err != nil {
		t.Fatalf("WriteFile(module): %v", err)
	}

	ca := &types.FullAnalysis{
		FilePath: casePath,
		CallChains: []*types.CallChain{{
			Steps: []types.CallStep{{
				IsWrapperCall: true,
				FullReceiver:  "adGroupPage.optimizationAndBiddingModule1MNBA",
				Callee:        "adGroupPage.optimizationAndBiddingModule1MNBA.vv_setStandardBtn",
				FuncName:      "vv_setStandardBtn",
			}},
		}},
	}

	deps := tracePropertyChainDeps(ca, nil)
	joined := strings.Join(deps, "\n")
	if !strings.Contains(joined, filepath.Clean(poPath)) {
		t.Fatalf("deps missing page object file: %v", deps)
	}
	if !strings.Contains(joined, filepath.Clean(modPath)) {
		t.Fatalf("deps missing module file: %v", deps)
	}
}

func TestHelperMethodAlreadyMigratedAcceptsMidsceneSuffixPerMethod(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "helper.ts")
	content := `
import { agent } from 'midscene';

export class OptimizationAndBiddingModule1MNBA {
  async vv_setStandardBtnMidscene() {
    await agent.aiTap("标准");
  }

  async legacyAction() {
    await ai?.action("旧逻辑");
  }
}
`
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if !isHelperMethodAlreadyMigrated(target, "vv_setStandardBtn") {
		t.Fatal("expected vv_setStandardBtn to be treated as migrated via Midscene suffix method")
	}
	if isHelperMethodAlreadyMigrated(target, "legacyAction") {
		t.Fatal("expected legacyAction to be treated as unmigrated because method body still uses NEP")
	}
}

func TestContainsMethodDefinitionSupportsReturnTypeAnnotations(t *testing.T) {
	text := `
export class CampaignBudgetRadio {
  async click(aiOptions?: AiActionOptions): Promise<void> {
    await super.click(aiOptions)
  }
}
`

	if !containsMethodDefinition(text, "click") {
		t.Fatal("expected method definition lookup to match class methods with return type annotations")
	}
}

func TestCollectCompileFixIndicesOnlyReturnsSuccessfulCompileFailures(t *testing.T) {
	results := []*types.MigrationResult{
		{CaseFile: "case-a.ts", Success: true},
		{CaseFile: "case-b.ts", Success: true},
		{CaseFile: "case-c.ts", Success: false},
		{CaseFile: "case-d.ts", Success: true},
	}
	verifyResults := []*types.VerifyResult{
		{CaseFile: "case-a.ts", CompileOK: false, CompileError: "missing module"},
		{CaseFile: "case-b.ts", CompileOK: true},
		{CaseFile: "case-c.ts", CompileOK: false, CompileError: "syntax error"},
		{CaseFile: "case-d.ts", CompileOK: false, CompileError: ""},
	}

	got := collectCompileFixIndices(results, verifyResults)
	if len(got) != 1 {
		t.Fatalf("collectCompileFixIndices returned %v, want [0]", got)
	}
	if got[0] != 0 {
		t.Fatalf("collectCompileFixIndices returned %v, want first failing successful result only", got)
	}
}

func TestBuildTypeScriptCompileCommandUsesTargetTsconfigAndLocalTSC(t *testing.T) {
	dir := t.TempDir()
	projectRoot := filepath.Join(dir, "target-repo")
	tsconfigPath := filepath.Join(projectRoot, "tsconfig.json")
	localTSC := filepath.Join(projectRoot, "node_modules", ".bin", "tsc")
	targetFile := filepath.Join(projectRoot, "e2e", "tests", "case.spec.ts")

	if err := os.MkdirAll(filepath.Dir(tsconfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(tsconfig): %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(localTSC), 0o755); err != nil {
		t.Fatalf("MkdirAll(localTSC): %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(targetFile): %v", err)
	}
	if err := os.WriteFile(tsconfigPath, []byte(`{"compilerOptions":{}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}
	if err := os.WriteFile(localTSC, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(localTSC): %v", err)
	}
	if err := os.WriteFile(targetFile, []byte(`export const ok = true;`), 0o644); err != nil {
		t.Fatalf("WriteFile(targetFile): %v", err)
	}

	results := []*types.MigrationResult{{TargetFile: targetFile, Success: true}}
	got := buildTypeScriptCompileCommand(projectRoot, results)
	want := localTSC + " --noEmit -p " + tsconfigPath
	if got != want {
		t.Fatalf("buildTypeScriptCompileCommand() = %q, want %q", got, want)
	}
}

func TestBuildMigrationPlanAggregatesHelperMethodsAcrossCases(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	engine := analyzer.NewEngine(cfg)

	e2eDir := filepath.Join(dir, "e2e")
	caseDir := filepath.Join(e2eDir, "tests", "brand")
	poDir := filepath.Join(e2eDir, "pages", "new_pages", "adGroupPage")
	modDir := filepath.Join(poDir, "module", "OptimizationAndBidding")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(caseDir): %v", err)
	}
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modDir): %v", err)
	}

	poPath := filepath.Join(poDir, "AdGroupPage.ts")
	poSource := `import { OptimizationAndBiddingModule1MNBA } from "./module/OptimizationAndBidding/OptimizationAndBiddingModule1MNBA";

export class AdGroupPage {
  public optimizationAndBiddingModule1MNBA: OptimizationAndBiddingModule1MNBA;
}
`
	if err := os.WriteFile(poPath, []byte(poSource), 0o644); err != nil {
		t.Fatalf("WriteFile(po): %v", err)
	}

	modulePath := filepath.Join(modDir, "OptimizationAndBiddingModule1MNBA.ts")
	moduleSource := `export class OptimizationAndBiddingModule1MNBA {
  async vv_setStandardBtn() {}
  async vv_goal_6s() {}
  async setBid(value: string) {}
}
`
	if err := os.WriteFile(modulePath, []byte(moduleSource), 0o644); err != nil {
		t.Fatalf("WriteFile(module): %v", err)
	}

	caseOnePath := filepath.Join(caseDir, "case-one.spec.ts")
	caseOneSource := `test("case one", async ({ adGroupPage }) => {
  await adGroupPage.optimizationAndBiddingModule1MNBA.vv_setStandardBtn();
  await adGroupPage.optimizationAndBiddingModule1MNBA.vv_goal_6s();
})`
	if err := os.WriteFile(caseOnePath, []byte(caseOneSource), 0o644); err != nil {
		t.Fatalf("WriteFile(caseOne): %v", err)
	}

	caseTwoPath := filepath.Join(caseDir, "case-two.spec.ts")
	caseTwoSource := `test("case two", async ({ adGroupPage }) => {
  await adGroupPage.optimizationAndBiddingModule1MNBA.setBid("1");
})`
	if err := os.WriteFile(caseTwoPath, []byte(caseTwoSource), 0o644); err != nil {
		t.Fatalf("WriteFile(caseTwo): %v", err)
	}

	targetPath := filepath.Join(modDir, cfg.Target.OutputDir, "OptimizationAndBiddingModule1MNBA"+cfg.Target.FileSuffix+".ts")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(targetDir): %v", err)
	}
	targetSource := `import { agent } from 'midscene';

export class OptimizationAndBiddingModule1MNBA {
  async vv_setStandardBtnMidscene() {
    await agent.aiTap("标准");
  }
}
`
	if err := os.WriteFile(targetPath, []byte(targetSource), 0o644); err != nil {
		t.Fatalf("WriteFile(target): %v", err)
	}

	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("AnalyzeDir: %v", err)
	}

	plan, err := buildMigrationPlan(cfg, engine, analyses, nil)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}

	if len(plan.ToExecuteHelpers) != 1 {
		var unresolved []types.UnresolvedHelper
		if len(plan.ToExecuteCases) > 0 {
			unresolved = plan.ToExecuteCases[0].UnresolvedHelpers
		}
		t.Fatalf("helper task count = %d, want 1 (cases=%d unresolved=%+v)", len(plan.ToExecuteHelpers), len(plan.ToExecuteCases), unresolved)
	}

	helper := plan.ToExecuteHelpers[0]
	if helper.FilePath != modulePath {
		t.Fatalf("helper source = %q, want %q", helper.FilePath, modulePath)
	}
	if helper.HelperPlan == nil {
		t.Fatal("expected helper plan metadata on helper task")
	}
	gotMethods := append([]string(nil), helper.HelperPlan.Methods...)
	sort.Strings(gotMethods)
	wantMethods := []string{"setBid", "vv_goal_6s"}
	if strings.Join(gotMethods, ",") != strings.Join(wantMethods, ",") {
		t.Fatalf("helper methods = %v, want %v", gotMethods, wantMethods)
	}
}

func TestBuildMigrationPlanMarksUnresolvedHelperMethodsOnCases(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	engine := analyzer.NewEngine(cfg)

	e2eDir := filepath.Join(dir, "e2e")
	caseDir := filepath.Join(e2eDir, "tests", "brand")
	poDir := filepath.Join(e2eDir, "pages", "new_pages", "adGroupPage")
	modDir := filepath.Join(poDir, "module", "OptimizationAndBidding")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(caseDir): %v", err)
	}
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modDir): %v", err)
	}

	if err := os.WriteFile(filepath.Join(poDir, "AdGroupPage.ts"), []byte(`import { OptimizationAndBiddingModule1MNBA } from "./module/OptimizationAndBidding/OptimizationAndBiddingModule1MNBA";
export class AdGroupPage {
  public optimizationAndBiddingModule1MNBA: OptimizationAndBiddingModule1MNBA;
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(po): %v", err)
	}

	if err := os.WriteFile(filepath.Join(modDir, "OptimizationAndBiddingModule1MNBA.ts"), []byte(`export class OptimizationAndBiddingModule1MNBA {
  async vv_setStandardBtn() {}
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(module): %v", err)
	}

	casePath := filepath.Join(caseDir, "case.spec.ts")
	caseSource := `test("case", async ({ adGroupPage }) => {
  await adGroupPage.optimizationAndBiddingModule1MNBA.setBid("1");
})`
	if err := os.WriteFile(casePath, []byte(caseSource), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}

	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("AnalyzeDir: %v", err)
	}

	plan, err := buildMigrationPlan(cfg, engine, analyses, nil)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}

	if len(plan.ToExecuteHelpers) != 0 {
		t.Fatalf("helper task count = %d, want 0 for unresolved-only receiver", len(plan.ToExecuteHelpers))
	}
	if len(plan.ToExecuteCases) != 1 {
		t.Fatalf("case task count = %d, want 1", len(plan.ToExecuteCases))
	}
	caseAnalysis := plan.ToExecuteCases[0]
	if len(caseAnalysis.UnresolvedHelpers) != 1 {
		t.Fatalf("unresolved helper count = %d, want 1", len(caseAnalysis.UnresolvedHelpers))
	}
	got := caseAnalysis.UnresolvedHelpers[0]
	if got.Receiver != "adGroupPage.optimizationAndBiddingModule1MNBA" {
		t.Fatalf("receiver = %q, want %q", got.Receiver, "adGroupPage.optimizationAndBiddingModule1MNBA")
	}
	if got.Method != "setBid" {
		t.Fatalf("method = %q, want %q", got.Method, "setBid")
	}
	if !got.ReceiverReachable {
		t.Fatal("expected unresolved helper to keep receiver reachable context")
	}
	if got.Reason == "" {
		t.Fatal("expected unresolved helper reason to be populated")
	}
}

func TestBuildMigrationPlanResolvesInheritedHelperMethods(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	engine := analyzer.NewEngine(cfg)

	e2eDir := filepath.Join(dir, "e2e")
	caseDir := filepath.Join(e2eDir, "tests", "campaign")
	poDir := filepath.Join(e2eDir, "pages", "new_pages", "campaignPage")
	componentDir := filepath.Join(poDir, "components")
	utilsDir := filepath.Join(e2eDir, "utils", "coreComponents")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(caseDir): %v", err)
	}
	if err := os.MkdirAll(componentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(componentDir): %v", err)
	}
	if err := os.MkdirAll(utilsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(utilsDir): %v", err)
	}

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	tsconfigSource := `{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@pages/*": ["e2e/pages/*"],
      "@utils/*": ["e2e/utils/*"]
    }
  }
}`
	if err := os.WriteFile(tsconfigPath, []byte(tsconfigSource), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	radioPath := filepath.Join(utilsDir, "Radio.ts")
	radioSource := `export class Radio {
  async click() {}
}`
	if err := os.WriteFile(radioPath, []byte(radioSource), 0o644); err != nil {
		t.Fatalf("WriteFile(radio): %v", err)
	}

	componentPath := filepath.Join(componentDir, "AdgroupBudgetRadio.ts")
	componentSource := `import { Radio } from '@utils/coreComponents/Radio';

export class AdgroupBudgetRadio extends Radio {}
`
	if err := os.WriteFile(componentPath, []byte(componentSource), 0o644); err != nil {
		t.Fatalf("WriteFile(component): %v", err)
	}

	poPath := filepath.Join(poDir, "CampaignPage.ts")
	poSource := `import { AdgroupBudgetRadio } from '@pages/new_pages/campaignPage/components/AdgroupBudgetRadio';

export class CampaignPage {
  adgroupBudgetRadio: AdgroupBudgetRadio;

  constructor(page: unknown) {
    this.adgroupBudgetRadio = new AdgroupBudgetRadio(page);
  }
}
`
	if err := os.WriteFile(poPath, []byte(poSource), 0o644); err != nil {
		t.Fatalf("WriteFile(po): %v", err)
	}

	casePath := filepath.Join(caseDir, "case.spec.ts")
	caseSource := `test("campaign", async ({ campaignPage }) => {
  await campaignPage.adgroupBudgetRadio.click();
})`
	if err := os.WriteFile(casePath, []byte(caseSource), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}

	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("AnalyzeDir: %v", err)
	}

	plan, err := buildMigrationPlan(cfg, engine, analyses, nil)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}

	if len(plan.ToExecuteHelpers) != 1 {
		var unresolved []types.UnresolvedHelper
		if len(plan.ToExecuteCases) > 0 {
			unresolved = plan.ToExecuteCases[0].UnresolvedHelpers
		}
		t.Fatalf("helper task count = %d, want 1 (cases=%d unresolved=%+v)", len(plan.ToExecuteHelpers), len(plan.ToExecuteCases), unresolved)
	}
	helper := plan.ToExecuteHelpers[0]
	if helper.FilePath != componentPath {
		t.Fatalf("helper source = %q, want %q", helper.FilePath, componentPath)
	}
	if helper.HelperPlan == nil {
		t.Fatal("expected helper plan metadata on helper task")
	}
	if strings.Join(helper.HelperPlan.Methods, ",") != "click" {
		t.Fatalf("helper methods = %v, want [click]", helper.HelperPlan.Methods)
	}
	if len(plan.ToExecuteCases) != 1 {
		t.Fatalf("case task count = %d, want 1", len(plan.ToExecuteCases))
	}
	if len(plan.ToExecuteCases[0].UnresolvedHelpers) != 0 {
		t.Fatalf("unexpected unresolved helpers: %+v", plan.ToExecuteCases[0].UnresolvedHelpers)
	}
}

func TestBuildMigrationPlanPromotesSharedMockDataImportOnce(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	cfg.Target.BaseDir = filepath.Join(dir, "target-repo")
	engine := analyzer.NewEngine(cfg)
	engine.SetSourceRepoRoot(dir)

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	tsconfigSource := `{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@testData/*": ["./e2e/test_data/*"]
    }
  }
}`
	if err := os.WriteFile(tsconfigPath, []byte(tsconfigSource), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	caseDir := filepath.Join(dir, "e2e", "tests", "brand")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(caseDir): %v", err)
	}

	mockDataPath := filepath.Join(dir, "e2e", "test_data", "mock-data.ts")
	if err := os.MkdirAll(filepath.Dir(mockDataPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(mockData dir): %v", err)
	}
	mockDataSource := `export const MockFeatureEnabledResponseBrandAuction = {
  status: 200,
  body: { data: { white_value: [] } },
};`
	if err := os.WriteFile(mockDataPath, []byte(mockDataSource), 0o644); err != nil {
		t.Fatalf("WriteFile(mockData): %v", err)
	}

	caseSource := `import { MockFeatureEnabledResponseBrandAuction } from '@testData/mock-data';

const BrandActionCreation = (page) =>
  page.intercept('/api/v3/i18n/account/features_enabled/', MockFeatureEnabledResponseBrandAuction);

before(async ({ page }) => {
  await BrandActionCreation(page);
});

test("case", async () => {
  expect(MockFeatureEnabledResponseBrandAuction.status).toBe(200);
});`

	caseOnePath := filepath.Join(caseDir, "case-one.spec.ts")
	if err := os.WriteFile(caseOnePath, []byte(caseSource), 0o644); err != nil {
		t.Fatalf("WriteFile(caseOne): %v", err)
	}
	caseTwoPath := filepath.Join(caseDir, "case-two.spec.ts")
	if err := os.WriteFile(caseTwoPath, []byte(strings.ReplaceAll(caseSource, `"case"`, `"case-two"`)), 0o644); err != nil {
		t.Fatalf("WriteFile(caseTwo): %v", err)
	}

	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("AnalyzeDir: %v", err)
	}

	plan, err := buildMigrationPlan(cfg, engine, analyses, nil)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}

	if len(plan.ToExecuteHelpers) != 1 {
		t.Fatalf("shared dependency task count = %d, want 1", len(plan.ToExecuteHelpers))
	}

	dep := plan.ToExecuteHelpers[0]
	if dep.TaskKind != "dependency" {
		t.Fatalf("task kind = %q, want %q", dep.TaskKind, "dependency")
	}
	if dep.FilePath != mockDataPath {
		t.Fatalf("dependency source = %q, want %q", dep.FilePath, mockDataPath)
	}

	wantTarget := filepath.Join(cfg.Target.BaseDir, "e2e", "test_data", "mock-data.ts")
	if dep.TargetPath != wantTarget {
		t.Fatalf("dependency target = %q, want %q", dep.TargetPath, wantTarget)
	}

	for _, ca := range plan.ToExecuteCases {
		found := false
		for _, p := range ca.Dependencies {
			if p == wantTarget {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("case %q missing shared dependency target %q in Dependencies: %v", ca.FilePath, wantTarget, ca.Dependencies)
		}
	}
}

func TestBuildMigrationPlanPromotesSharedHookThroughBarrelImport(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	cfg.Target.BaseDir = filepath.Join(dir, "target-repo")
	engine := analyzer.NewEngine(cfg)
	engine.SetSourceRepoRoot(dir)

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@utils/*": ["./e2e/utils/*"]
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	barrelPath := filepath.Join(dir, "e2e", "utils", "index.ts")
	if err := os.MkdirAll(filepath.Dir(barrelPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(barrel): %v", err)
	}
	if err := os.WriteFile(barrelPath, []byte(`export * from './describe-before';`), 0o644); err != nil {
		t.Fatalf("WriteFile(barrel): %v", err)
	}

	concretePath := filepath.Join(dir, "e2e", "utils", "describe-before", "index.ts")
	if err := os.MkdirAll(filepath.Dir(concretePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(concrete): %v", err)
	}
	if err := os.WriteFile(concretePath, []byte(`export const commonAfter = async () => {
  console.log("real hook");
};`), 0o644); err != nil {
		t.Fatalf("WriteFile(concrete): %v", err)
	}

	caseDir := filepath.Join(dir, "e2e", "tests", "brand")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(caseDir): %v", err)
	}
	casePath := filepath.Join(caseDir, "case.spec.ts")
	if err := os.WriteFile(casePath, []byte(`import { after, test } from '@playwright/test';
import { commonAfter } from '@utils/index';

after(commonAfter);
test("case", async () => {});`), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}

	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("AnalyzeDir: %v", err)
	}

	plan, err := buildMigrationPlan(cfg, engine, analyses, nil)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}

	if len(plan.ToExecuteHelpers) != 1 {
		t.Fatalf("shared dependency task count = %d, want 1", len(plan.ToExecuteHelpers))
	}

	dep := plan.ToExecuteHelpers[0]
	if dep.TaskKind != "dependency" {
		t.Fatalf("task kind = %q, want %q", dep.TaskKind, "dependency")
	}
	if dep.FilePath != concretePath {
		t.Fatalf("dependency source = %q, want %q", dep.FilePath, concretePath)
	}

	wantTarget := filepath.Join(cfg.Target.BaseDir, "e2e", "utils", "describe-before", "index.ts")
	if dep.TargetPath != wantTarget {
		t.Fatalf("dependency target = %q, want %q", dep.TargetPath, wantTarget)
	}

	if len(plan.ToExecuteCases) != 1 {
		t.Fatalf("case task count = %d, want 1", len(plan.ToExecuteCases))
	}
	found := false
	for _, p := range plan.ToExecuteCases[0].Dependencies {
		if p == wantTarget {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("case dependencies = %v, want to include %q", plan.ToExecuteCases[0].Dependencies, wantTarget)
	}
}

func TestBuildMigrationPlanDiscoversDirectInstantiatedHelperMethods(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	engine := analyzer.NewEngine(cfg)

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@pages/*": ["./e2e/pages/*"]
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	helperFile := filepath.Join(dir, "e2e", "pages", "new_pages", "creativePage", "module", "AdNameModule", "AdNameModule.ts")
	if err := os.MkdirAll(filepath.Dir(helperFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(helper): %v", err)
	}
	if err := os.WriteFile(helperFile, []byte(`export class AdNameModule {
  constructor(page: unknown) {
    void page;
  }

  async setAdName(adName: string) {
    console.log(adName);
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(helper): %v", err)
	}

	caseDir := filepath.Join(dir, "e2e", "tests", "brand")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(caseDir): %v", err)
	}
	casePath := filepath.Join(caseDir, "case.spec.ts")
	if err := os.WriteFile(casePath, []byte(`import { AdNameModule } from "@pages/new_pages/creativePage/module/AdNameModule/AdNameModule";

test("case", async ({ page }) => {
  const adNameModule = new AdNameModule(page);
  await adNameModule.setAdName("creative");
});`), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}

	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("AnalyzeDir: %v", err)
	}

	plan, err := buildMigrationPlan(cfg, engine, analyses, nil)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}

	if len(plan.ToExecuteHelpers) != 1 {
		var unresolved []types.UnresolvedHelper
		if len(plan.ToExecuteCases) > 0 {
			unresolved = plan.ToExecuteCases[0].UnresolvedHelpers
		}
		t.Fatalf("helper task count = %d, want 1 (cases=%d unresolved=%+v)", len(plan.ToExecuteHelpers), len(plan.ToExecuteCases), unresolved)
	}
	helper := plan.ToExecuteHelpers[0]
	if helper.FilePath != helperFile {
		t.Fatalf("helper source = %q, want %q", helper.FilePath, helperFile)
	}
	if helper.HelperPlan == nil {
		t.Fatal("expected helper plan metadata on helper task")
	}
	if strings.Join(helper.HelperPlan.Methods, ",") != "setAdName" {
		t.Fatalf("helper methods = %v, want [setAdName]", helper.HelperPlan.Methods)
	}
	if len(plan.ToExecuteCases) != 1 {
		t.Fatalf("case task count = %d, want 1", len(plan.ToExecuteCases))
	}
	if len(plan.ToExecuteCases[0].UnresolvedHelpers) != 0 {
		t.Fatalf("unexpected unresolved helpers: %+v", plan.ToExecuteCases[0].UnresolvedHelpers)
	}
}

func TestResolveHelperReceiverCandidateRejectsDirectoryModulePath(t *testing.T) {
	dir := t.TempDir()

	candidate := &helperReceiverCandidate{
		Receiver:       "adNameModule",
		PageObjectFile: dir,
		MethodsByCase: map[string]map[string]struct{}{
			filepath.Join(dir, "case.spec.ts"): {
				"setAdName": {},
			},
		},
	}

	_, err := resolveHelperReceiverCandidate(candidate, nil, analyzer.NewEngine(config.DefaultConfig()))
	if err == nil {
		t.Fatal("expected directory path to be rejected")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("error = %q, want to mention directory", err)
	}
}

func TestBuildMigrationPlanSkipsUpToDateSharedMockDataDependency(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	cfg.Target.BaseDir = filepath.Join(dir, "target-repo")
	engine := analyzer.NewEngine(cfg)
	engine.SetSourceRepoRoot(dir)

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@testData/*": ["./e2e/test_data/*"]
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	caseDir := filepath.Join(dir, "e2e", "tests")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(caseDir): %v", err)
	}

	mockDataPath := filepath.Join(dir, "e2e", "test_data", "mock-data.ts")
	if err := os.MkdirAll(filepath.Dir(mockDataPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(mockData dir): %v", err)
	}
	mockDataSource := `export const MockFeatureEnabledResponseBrandAuction = { status: 200 };`
	if err := os.WriteFile(mockDataPath, []byte(mockDataSource), 0o644); err != nil {
		t.Fatalf("WriteFile(mockData): %v", err)
	}

	casePath := filepath.Join(caseDir, "case.spec.ts")
	if err := os.WriteFile(casePath, []byte(`import { MockFeatureEnabledResponseBrandAuction } from '@testData/mock-data';
test("case", async () => {
  expect(MockFeatureEnabledResponseBrandAuction.status).toBe(200);
});`), 0o644); err != nil {
		t.Fatalf("WriteFile(case): %v", err)
	}

	store, err := executor.NewStateStore(dir)
	if err != nil {
		t.Fatalf("NewStateStore: %v", err)
	}
	runID := "run-shared-dep"
	now := time.Now()
	if err := store.StartRun(runID, dir, cfg.Target.BaseDir, 1, now); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	targetPath := filepath.Join(cfg.Target.BaseDir, "e2e", "test_data", "mock-data.ts")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(targetDir): %v", err)
	}
	if err := os.WriteFile(targetPath, []byte(mockDataSource), 0o644); err != nil {
		t.Fatalf("WriteFile(target): %v", err)
	}

	hashBytes, err := os.ReadFile(mockDataPath)
	if err != nil {
		t.Fatalf("ReadFile(mockData): %v", err)
	}
	sum := sha256.Sum256(hashBytes)
	taskKey := defaultTaskKey("dependency", mockDataPath)
	if err := store.RecordTaskResult(runID, taskKey, mockDataPath, "completed", "", "dependency", hex.EncodeToString(sum[:]), targetPath, now.Add(time.Second)); err != nil {
		t.Fatalf("RecordTaskResult: %v", err)
	}
	if err := store.CompleteRun(runID, "completed", now.Add(2*time.Second)); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("AnalyzeDir: %v", err)
	}

	plan, err := buildMigrationPlan(cfg, engine, analyses, store)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}

	if len(plan.ToExecuteHelpers) != 0 {
		t.Fatalf("shared dependency task count = %d, want 0 when up-to-date", len(plan.ToExecuteHelpers))
	}

	foundSkipped := false
	for _, meta := range plan.Skipped {
		if meta.TaskKey == taskKey && meta.Kind == "dependency" {
			foundSkipped = true
			break
		}
	}
	if !foundSkipped {
		t.Fatalf("expected dependency task %q to be skipped, got %#v", taskKey, plan.Skipped)
	}
}

func TestRunStartStreamsAnalyzeProgressBeforeGenerate(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	cfg.Execution.MaxJobs = 1
	cfg.Execution.RetryLimit = 0
	runtime := NewRuntime(cfg)
	runtime.preflightCheck = func(string) error { return nil }
	runtime.newPromptExecutor = func(tool, workDir string) executor.PromptExecutor {
		return stubPromptExecutor{}
	}

	sourcePath := filepath.Join(dir, "case.spec.ts")
	if err := os.WriteFile(sourcePath, []byte(`test("case", async () => {
  await page.click("#submit")
})`), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	targetPath := filepath.Join(dir, cfg.Target.OutputDir, "case.spec"+cfg.Target.FileSuffix+".ts")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte(`export async function migrated(agent) {
  await agent.aiTap("提交按钮")
}`), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	var events []WorkflowEvent
	_, err := runtime.RunStart(context.Background(), dir, "", func(event WorkflowEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("RunStart: %v", err)
	}

	findIndex := func(match func(WorkflowEvent) bool) int {
		t.Helper()
		for i, event := range events {
			if match(event) {
				return i
			}
		}
		return -1
	}

	analyzeStart := findIndex(func(event WorkflowEvent) bool {
		return event.Stage == "analyze" && event.Message == "分析目录结构"
	})
	if analyzeStart < 0 {
		t.Fatal("missing initial analyze event")
	}

	progressIdx := findIndex(func(event WorkflowEvent) bool {
		return event.Stage == "analyze" && event.Current == 1 && event.Total == 1 && event.CurrentFile == sourcePath
	})
	if progressIdx < 0 {
		t.Fatal("missing analyze progress event")
	}

	generateIdx := findIndex(func(event WorkflowEvent) bool {
		return event.Stage == "generate" && strings.Contains(event.Message, "生成迁移计划")
	})
	if generateIdx < 0 {
		t.Fatal("missing generate event")
	}

	if !(analyzeStart < progressIdx && progressIdx < generateIdx) {
		t.Fatalf("event order invalid: analyzeStart=%d progress=%d generate=%d", analyzeStart, progressIdx, generateIdx)
	}
}

func TestRunStartAnalyzeProgressExcludesCrossRepoTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	cfg.Execution.MaxJobs = 1
	cfg.Execution.RetryLimit = 0
	runtime := NewRuntime(cfg)
	runtime.preflightCheck = func(string) error { return nil }
	runtime.newPromptExecutor = func(tool, workDir string) executor.PromptExecutor {
		return stubPromptExecutor{}
	}

	sourceDir := filepath.Join(dir, "e2e", "tests")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	sourcePath := filepath.Join(sourceDir, "case.spec.ts")
	if err := os.WriteFile(sourcePath, []byte(`test("case", async () => {
  await page.click("#submit")
})`), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	targetBaseDir := filepath.Join(dir, "target-repo")
	targetNoisePath := filepath.Join(targetBaseDir, "noise.spec.ts")
	if err := os.MkdirAll(filepath.Dir(targetNoisePath), 0o755); err != nil {
		t.Fatalf("mkdir target repo: %v", err)
	}
	if err := os.WriteFile(targetNoisePath, []byte(`test("noise", async () => {})`), 0o644); err != nil {
		t.Fatalf("write target noise file: %v", err)
	}

	targetPath := filepath.Join(targetBaseDir, "e2e", "tests", "case.spec.ts")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir cross-repo target dir: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte(`export async function migrated(agent) {
  await agent.aiTap("提交按钮")
}`), 0o644); err != nil {
		t.Fatalf("write cross-repo target file: %v", err)
	}

	var events []WorkflowEvent
	_, err := runtime.RunStart(context.Background(), dir, targetBaseDir, func(event WorkflowEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("RunStart: %v", err)
	}

	progressEvents := make([]WorkflowEvent, 0)
	for _, event := range events {
		if event.Stage == "analyze" && event.Total > 0 {
			progressEvents = append(progressEvents, event)
		}
	}
	if len(progressEvents) != 1 {
		t.Fatalf("analyze progress event count = %d, want 1", len(progressEvents))
	}
	if progressEvents[0].Total != 1 {
		t.Fatalf("analyze progress total = %d, want 1", progressEvents[0].Total)
	}
	if progressEvents[0].CurrentFile != sourcePath {
		t.Fatalf("analyze progress currentFile = %q, want %q", progressEvents[0].CurrentFile, sourcePath)
	}
}

type stubPromptExecutor struct{}

func (stubPromptExecutor) Execute(ctx context.Context, prompt string) (*types.CocoOutput, error) {
	return &types.CocoOutput{
		Success:  true,
		ExitCode: 0,
		Duration: time.Millisecond,
		Output:   prompt,
	}, nil
}
