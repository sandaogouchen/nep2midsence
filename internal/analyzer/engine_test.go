package analyzer

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/types"
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

func TestAnalyzeDirWithProgressSkipsOutputDirectoryAndReportsStableTotals(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "case.spec.ts")
	if err := os.WriteFile(sourcePath, []byte(`test("case", async () => {
  await page.click("#submit")
})`), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	skippedDir := filepath.Join(dir, cfg.Target.OutputDir)
	if err := os.MkdirAll(skippedDir, 0o755); err != nil {
		t.Fatalf("mkdir skipped dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skippedDir, "case_converted.ts"), []byte(`test("skip", async () => {})`), 0o644); err != nil {
		t.Fatalf("write skipped file: %v", err)
	}

	type progressEvent struct {
		current int
		total   int
		file    string
	}
	var events []progressEvent

	results, err := engine.AnalyzeDirWithProgress(dir, func(current, total int, filePath string) {
		events = append(events, progressEvent{current: current, total: total, file: filePath})
	})
	if err != nil {
		t.Fatalf("AnalyzeDirWithProgress: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	wantEvents := []progressEvent{{current: 1, total: 1, file: sourcePath}}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("progress events = %#v, want %#v", events, wantEvents)
	}
}

func TestAnalyzeDirWithProgressContinuesAfterFileFailure(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Source.Extensions = []string{".go"}
	engine := NewEngine(cfg)

	dir := t.TempDir()
	badPath := filepath.Join(dir, "broken.go")
	if err := os.WriteFile(badPath, []byte("package broken\nfunc TestBroken( {\n"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}
	goodPath := filepath.Join(dir, "valid.go")
	if err := os.WriteFile(goodPath, []byte("package broken\nfunc Helper() {}\n"), 0o644); err != nil {
		t.Fatalf("write good file: %v", err)
	}

	type progressEvent struct {
		current int
		total   int
		file    string
	}
	var events []progressEvent

	results, err := engine.AnalyzeDirWithProgress(dir, func(current, total int, filePath string) {
		events = append(events, progressEvent{current: current, total: total, file: filePath})
	})
	if err != nil {
		t.Fatalf("AnalyzeDirWithProgress: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1 analyzed file", len(results))
	}
	wantEvents := []progressEvent{
		{current: 1, total: 2, file: badPath},
		{current: 2, total: 2, file: goodPath},
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("progress events = %#v, want %#v", events, wantEvents)
	}
	if results[0].FilePath != goodPath {
		t.Fatalf("analyzed file = %q, want %q", results[0].FilePath, goodPath)
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

func TestAnalyzeFileClassifiesBusinessAndInfrastructureOwners(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.WrapperFilter.ForceInfraCallPatterns = []string{`.*\.editAdSubmitBtn\.click$`}
	engine := NewEngine(cfg)

	dir := t.TempDir()
	pageObjectPath := filepath.Join(dir, "ListPage.ts")
	casePath := filepath.Join(dir, "sample.spec.ts")

	pageObjectSource := `export default class ListPage {
  commonActions = {}
  editAdSubmitBtn = {}
}
`
	caseSource := `import ListPage from "./ListPage"

const listPage = new ListPage()

test("owner classification", async ({ page }) => {
  await page.sessionStorage.set("k", "v")
  await listPage.commonActions.editCampaign2("campaignName", "campaignId")
  await listPage.editAdSubmitBtn.click()
  console.log("debug")
})
`
	if err := os.WriteFile(pageObjectPath, []byte(pageObjectSource), 0o644); err != nil {
		t.Fatalf("write page object: %v", err)
	}
	if err := os.WriteFile(casePath, []byte(caseSource), 0o644); err != nil {
		t.Fatalf("write case file: %v", err)
	}

	result, err := engine.AnalyzeFile(casePath)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	steps := flattenCallSteps(result)
	assertStep := func(fragment string) types.CallStep {
		t.Helper()
		for _, st := range steps {
			if strings.Contains(st.Callee, fragment) {
				return st
			}
		}
		t.Fatalf("did not find step containing %q", fragment)
		return types.CallStep{}
	}

	pageStorage := assertStep("page.sessionStorage.set")
	if pageStorage.OwnerKind != "infrastructure" {
		t.Fatalf("page.sessionStorage.set OwnerKind = %q, want infrastructure", pageStorage.OwnerKind)
	}
	if pageStorage.IsWrapperCall {
		t.Fatalf("page.sessionStorage.set IsWrapperCall = true, want false")
	}

	businessWrapper := assertStep("listPage.commonActions.editCampaign2")
	if businessWrapper.OwnerKind != "business" {
		t.Fatalf("listPage.commonActions.editCampaign2 OwnerKind = %q, want business", businessWrapper.OwnerKind)
	}
	if !businessWrapper.IsWrapperCall {
		t.Fatalf("listPage.commonActions.editCampaign2 IsWrapperCall = false, want true")
	}
	if businessWrapper.OwnerRoot != "listPage" {
		t.Fatalf("listPage.commonActions.editCampaign2 OwnerRoot = %q, want listPage", businessWrapper.OwnerRoot)
	}
	if businessWrapper.OwnerFile != pageObjectPath {
		t.Fatalf("listPage.commonActions.editCampaign2 OwnerFile = %q, want %q", businessWrapper.OwnerFile, pageObjectPath)
	}

	elementClick := assertStep("listPage.editAdSubmitBtn.click")
	if elementClick.OwnerKind != "infrastructure" {
		t.Fatalf("listPage.editAdSubmitBtn.click OwnerKind = %q, want infrastructure", elementClick.OwnerKind)
	}
	if elementClick.IsWrapperCall {
		t.Fatalf("listPage.editAdSubmitBtn.click IsWrapperCall = true, want false")
	}

	consoleLog := assertStep("console.log")
	if consoleLog.OwnerKind != "infrastructure" {
		t.Fatalf("console.log OwnerKind = %q, want infrastructure", consoleLog.OwnerKind)
	}
	if consoleLog.IsWrapperCall {
		t.Fatalf("console.log IsWrapperCall = true, want false")
	}
}

func TestAnalyzeFileClassifiesForceInfraMethodWithoutReceiver(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "sample.spec.ts")
	caseSource := `test("force infra method", async () => {
  await waitForPageLoadStable()
})
`
	if err := os.WriteFile(casePath, []byte(caseSource), 0o644); err != nil {
		t.Fatalf("write case file: %v", err)
	}

	result, err := engine.AnalyzeFile(casePath)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	step := types.CallStep{}
	found := false
	for _, candidate := range flattenCallSteps(result) {
		if candidate.Callee == "waitForPageLoadStable" {
			step = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("did not find waitForPageLoadStable in extracted steps")
	}
	if step.OwnerKind != "infrastructure" {
		t.Fatalf("OwnerKind = %q, want infrastructure", step.OwnerKind)
	}
	if step.OwnerSource != "config_force_infra_method" {
		t.Fatalf("OwnerSource = %q, want config_force_infra_method", step.OwnerSource)
	}
	if step.OwnerRoot != "" {
		t.Fatalf("OwnerRoot = %q, want empty", step.OwnerRoot)
	}
	if step.IsWrapperCall {
		t.Fatalf("IsWrapperCall = true, want false")
	}
}

func TestAnalyzeFileClassifiesBusinessWrappersInsideCommonIt(t *testing.T) {
	cfg := config.DefaultConfig()
	if !matchesAnyPattern("campaignPage", cfg.WrapperFilter.KnownBusinessNamePatterns) {
		t.Fatalf("campaignPage should match business patterns: %#v", cfg.WrapperFilter.KnownBusinessNamePatterns)
	}
	engine := NewEngine(cfg)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "sample.spec.ts")
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
		t.Fatalf("write ts file: %v", err)
	}

	result, err := engine.AnalyzeFile(casePath)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	steps := flattenCallSteps(result)
	assertWrapper := func(callee string) {
		t.Helper()
		for _, st := range steps {
			if st.Callee == callee {
				if st.OwnerKind != "business" {
					t.Fatalf("%s OwnerKind = %q, want business (ownerRoot=%q ownerSource=%q fullReceiver=%q)", callee, st.OwnerKind, st.OwnerRoot, st.OwnerSource, st.FullReceiver)
				}
				if !st.IsWrapperCall {
					t.Fatalf("%s IsWrapperCall = false, want true", callee)
				}
				return
			}
		}
		for _, st := range steps {
			t.Logf("step callee=%s ownerKind=%s wrapper=%v line=%d", st.Callee, st.OwnerKind, st.IsWrapperCall, st.Line)
		}
		t.Fatalf("did not find step %q", callee)
	}

	assertWrapper("campaignPage.campaignNameModule.setCampaignName")
	assertWrapper("campaignPage.campaignNameModule1MN.setCBOBudgetStrategy")
	assertWrapper("adGroupPage.optimizationAndBiddingModule1MNBA.vv_setAfterViewBtn")
}

func TestAnalyzeFileInfersOwnerFilesForCommonItFixturePages(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	dir := t.TempDir()
	e2eDir := filepath.Join(dir, "e2e")
	casePath := filepath.Join(e2eDir, "tests", "new_tests", "brand", "video-view", "sample.spec.ts")
	campaignPagePath := filepath.Join(e2eDir, "pages", "new_pages", "campaignPage", "CampaignPage.ts")
	adGroupPagePath := filepath.Join(e2eDir, "pages", "new_pages", "adGroupPage", "AdGroupPage.ts")
	if err := os.MkdirAll(filepath.Dir(casePath), 0o755); err != nil {
		t.Fatalf("mkdir case dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(campaignPagePath), 0o755); err != nil {
		t.Fatalf("mkdir campaign page dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(adGroupPagePath), 0o755); err != nil {
		t.Fatalf("mkdir adgroup page dir: %v", err)
	}
	if err := os.WriteFile(campaignPagePath, []byte(`export class CampaignPage {}`), 0o644); err != nil {
		t.Fatalf("write campaign page: %v", err)
	}
	if err := os.WriteFile(adGroupPagePath, []byte(`export class AdGroupPage {}`), 0o644); err != nil {
		t.Fatalf("write adgroup page: %v", err)
	}
	caseSource := `import { commonIt } from "@utils/nep_utils/CommonIt/CommonIt"

describe("x", () => {
  it("case", commonIt("mock-url", async ({ campaignPage, adGroupPage }) => {
    await campaignPage.campaignNameModule.setCampaignName("demo");
    await adGroupPage.optimizationAndBiddingModule1MNBA.setBid("1");
  }))
})
`
	if err := os.WriteFile(casePath, []byte(caseSource), 0o644); err != nil {
		t.Fatalf("write case file: %v", err)
	}

	result, err := engine.AnalyzeFile(casePath)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	assertOwnerFile := func(callee, want string) {
		t.Helper()
		for _, st := range flattenCallSteps(result) {
			if st.Callee == callee {
				if st.OwnerFile != want {
					t.Fatalf("%s OwnerFile = %q, want %q", callee, st.OwnerFile, want)
				}
				return
			}
		}
		t.Fatalf("did not find step %q", callee)
	}

	assertOwnerFile("campaignPage.campaignNameModule.setCampaignName", campaignPagePath)
	assertOwnerFile("adGroupPage.optimizationAndBiddingModule1MNBA.setBid", adGroupPagePath)
}

func TestAnalyzeFileDeduplicatesAwaitedCalls(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "sample.spec.ts")
	caseSource := `test("awaited call", async () => {
  await listPage.commonActions.editAdGroup2("adgroup", "cid");
})`
	if err := os.WriteFile(casePath, []byte(caseSource), 0o644); err != nil {
		t.Fatalf("write case file: %v", err)
	}

	result, err := engine.AnalyzeFile(casePath)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	count := 0
	for _, st := range flattenCallSteps(result) {
		if st.Callee == "listPage.commonActions.editAdGroup2" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("awaited wrapper call count = %d, want 1", count)
	}
}

func flattenCallSteps(result *types.FullAnalysis) []types.CallStep {
	var steps []types.CallStep
	if result == nil {
		return steps
	}
	for _, chain := range result.CallChains {
		steps = append(steps, chain.Steps...)
	}
	return steps
}
