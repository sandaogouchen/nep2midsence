package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	deps := tracePropertyChainDeps(ca)
	joined := strings.Join(deps, "\n")
	if !strings.Contains(joined, filepath.Clean(poPath)) {
		t.Fatalf("deps missing page object file: %v", deps)
	}
	if !strings.Contains(joined, filepath.Clean(modPath)) {
		t.Fatalf("deps missing module file: %v", deps)
	}
}
