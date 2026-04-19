package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/executor"
)

func TestModelRejectsPlainTextInput(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})
	model.commandInput.SetValue("hello")

	next, _ := model.Update(submitCommandMsg{})
	updated := next.(Model)

	if updated.lastError == "" {
		t.Fatal("lastError = empty, want slash-command rejection")
	}
}

func TestModelRoutesHelpCommand(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})
	model.commandInput.SetValue("/help")

	next, _ := model.Update(submitCommandMsg{})
	updated := next.(Model)

	if updated.activeView != viewHelp {
		t.Fatalf("activeView = %v, want %v", updated.activeView, viewHelp)
	}
}

func TestModelModelCommandSwitchesExecutorTool(t *testing.T) {
	cfg := config.DefaultConfig()
	model := NewModel(cfg, NewNoopRuntime(), Options{})
	model.commandInput.SetValue("/model codex")

	next, _ := model.Update(submitCommandMsg{})
	updated := next.(Model)

	if cfg.Execution.Tool != "codex" {
		t.Fatalf("Execution.Tool = %q, want %q", cfg.Execution.Tool, "codex")
	}
	if updated.lastInfo == "" {
		t.Fatal("lastInfo = empty, want switch confirmation")
	}
}

func TestModelClearOnlyClearsLogs(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})
	model.logs = []string{"line 1", "line 2"}
	model.lastError = "boom"
	model.commandInput.SetValue("/clear")

	next, _ := model.Update(submitCommandMsg{})
	updated := next.(Model)

	if len(updated.logs) != 0 {
		t.Fatalf("logs length = %d, want 0", len(updated.logs))
	}
	if updated.lastError != "" {
		t.Fatalf("lastError = %q, want empty", updated.lastError)
	}
}

func TestModelQuitCommandReturnsQuit(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})
	model.commandInput.SetValue("/quit")

	next, cmd := model.Update(submitCommandMsg{})
	updated := next.(Model)

	if !updated.quitting {
		t.Fatal("quitting = false, want true")
	}
	if cmd == nil {
		t.Fatal("quit command returned nil tea.Cmd")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("quit cmd message = %#v, want tea.Quit", msg)
	}
}

func TestModelCtrlCQuits(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := next.(Model)

	if !updated.quitting {
		t.Fatal("quitting = false, want true after ctrl+c")
	}
	if cmd == nil {
		t.Fatal("ctrl+c returned nil tea.Cmd")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("ctrl+c cmd message = %#v, want tea.Quit", msg)
	}
}

func TestModelSmokeViewDoesNotPanic(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})
	_ = model.Init()
	_ = model.View()
}

func TestModelViewRendersCompactCommandBar(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})

	view := model.View()

	if !strings.Contains(view, "Command") {
		t.Fatal("view missing command label")
	}
	if !strings.Contains(view, "/help") {
		t.Fatal("view missing slash command hint")
	}
	if strings.Contains(view, "Command Input") {
		t.Fatal("view should not render legacy command input title")
	}
	if strings.Contains(view, "Type a slash command like /help or /start") {
		t.Fatal("view should not render legacy command input helper copy")
	}
	if strings.Contains(view, "┌") || strings.Contains(view, "┐") {
		t.Fatal("view should not render legacy bordered command frame")
	}
}

func TestBeginWorkflowUsesAnalyzeRuntime(t *testing.T) {
	runtime := &countingRuntime{}
	model := NewModel(config.DefaultConfig(), runtime, Options{})
	model.pickerMode = modeAnalyze

	next, cmd := model.beginWorkflow(".")
	updated := next.(Model)
	if cmd == nil {
		t.Fatal("beginWorkflow returned nil command")
	}

	for i := 0; i < 3 && cmd != nil; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		next, cmd = updated.Update(msg)
		updated = next.(Model)
	}

	if runtime.analyzeCalls != 1 {
		t.Fatalf("analyzeCalls = %d, want 1", runtime.analyzeCalls)
	}
	if runtime.startCalls != 0 {
		t.Fatalf("startCalls = %d, want 0", runtime.startCalls)
	}
}

func TestDirectoryPickerFiltersDirectoriesFromSearchInput(t *testing.T) {
	runtime := &countingRuntime{
		dirs: []string{"./alpha", "./beta-service", "./gamma"},
	}
	model := NewModel(config.DefaultConfig(), runtime, Options{WorkDir: "."})

	next, _ := model.openDirectoryPicker(modeStart)
	updated := next.(Model)

	for _, key := range []string{"b", "e", "t", "a"} {
		next, _ = updated.Update(keyMsg(key))
		updated = next.(Model)
	}

	if updated.directorySearchInput.Value() != "beta" {
		t.Fatalf("directorySearchInput = %q, want %q", updated.directorySearchInput.Value(), "beta")
	}
	assertVisibleDirectories(t, updated.directoryList, []string{"./beta-service"})
}

func TestDirectoryPickerKeepsListNavigationAfterSearch(t *testing.T) {
	runtime := &countingRuntime{
		dirs: []string{"./alpha", "./beta-one", "./beta-two"},
	}
	model := NewModel(config.DefaultConfig(), runtime, Options{WorkDir: "."})

	next, _ := model.openDirectoryPicker(modeStart)
	updated := next.(Model)

	for _, key := range []string{"b", "e", "t", "a"} {
		next, _ = updated.Update(keyMsg(key))
		updated = next.(Model)
	}
	next, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated = next.(Model)

	// Enter selects the source directory; for modeStart this transitions to target picker.
	next, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = next.(Model)
	if updated.activeView != viewTargetDirectory {
		t.Fatalf("activeView = %q, want %q", updated.activeView, viewTargetDirectory)
	}

	// Tab confirms the target directory and triggers the workflow.
	next, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated = next.(Model)
	if cmd == nil {
		t.Fatal("tab in target picker returned nil tea.Cmd")
	}

	for i := 0; i < 3 && cmd != nil; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		next, cmd = updated.Update(msg)
		updated = next.(Model)
	}

	if runtime.startCalls != 1 {
		t.Fatalf("startCalls = %d, want 1", runtime.startCalls)
	}
	if updated.lastResult == nil {
		t.Fatal("lastResult = nil, want workflow result")
	}
	if updated.lastResult.Dir != "./beta-two" {
		t.Fatalf("lastResult.Dir = %q, want %q", updated.lastResult.Dir, "./beta-two")
	}
}

func TestTargetDirectoryPickerStartsFromParentOfWorkDirAndIgnoresConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Target.BaseDir = "/should/not/be/used"

	model := NewModel(cfg, &countingRuntime{}, Options{
		WorkDir: "/Users/bytedance/tt4b_ai_e2e/",
	})

	next, _ := model.openTargetDirectoryPicker()
	updated := next.(Model)

	want := filepath.Clean("/Users/bytedance")
	if updated.targetBrowsePath != want {
		t.Fatalf("targetBrowsePath = %q, want %q", updated.targetBrowsePath, want)
	}
}

func TestDirectoryPickerMatchesPathTokensAcrossNestedDirectories(t *testing.T) {
	runtime := &countingRuntime{
		dirs: []string{
			"/Users/bytedance/tt4b_creation_e2e/e2e/tests/new_tests/brand/community-interaction",
			"/Users/bytedance/tt4b_creation_e2e/e2e/tests/legacy/payment",
		},
	}
	model := NewModel(config.DefaultConfig(), runtime, Options{WorkDir: "."})

	next, _ := model.openDirectoryPicker(modeStart)
	updated := next.(Model)

	for _, key := range []string{"n", "e", "w", "_", "t", "e", "s", "t", "/", "b", "r", "a", "n", "d"} {
		next, _ = updated.Update(keyMsg(key))
		updated = next.(Model)
	}

	assertVisibleDirectories(t, updated.directoryList, []string{
		"/Users/bytedance/tt4b_creation_e2e/e2e/tests/new_tests/brand/community-interaction",
	})
}

func TestRuntimeEventAppendsStructuredLogLines(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})

	next, _ := model.Update(runtimeEventMsg{event: WorkflowEvent{
		Stage:    "execute",
		LogLines: []string{"case-a", "coco: success"},
	}})
	updated := next.(Model)

	if len(updated.logs) < 2 {
		t.Fatalf("logs length = %d, want at least 2", len(updated.logs))
	}
	got := updated.logs[len(updated.logs)-2:]
	want := []string{"case-a", "coco: success"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("logs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRuntimeEventResetsCurrentProgressWhenStageChangesWithNewTotal(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})
	model.currentStage = "analyze"
	model.progressCurrent = 27
	model.progressTotal = 42

	next, _ := model.Update(runtimeEventMsg{event: WorkflowEvent{
		Stage:     "generate",
		Total:     9,
		Successes: 2,
		Failures:  1,
	}})
	updated := next.(Model)

	if updated.progressCurrent != 0 {
		t.Fatalf("progressCurrent = %d, want 0 after stage change", updated.progressCurrent)
	}
	if updated.progressTotal != 9 {
		t.Fatalf("progressTotal = %d, want 9", updated.progressTotal)
	}

	next, _ = updated.Update(runtimeEventMsg{event: WorkflowEvent{
		Stage:   "execute",
		Message: "开始执行迁移",
	}})
	updated = next.(Model)

	if updated.progressCurrent != 0 {
		t.Fatalf("progressCurrent after execute start = %d, want 0", updated.progressCurrent)
	}
	if updated.progressTotal != 9 {
		t.Fatalf("progressTotal after execute start = %d, want 9", updated.progressTotal)
	}
	if updated.successes != 2 {
		t.Fatalf("successes = %d, want 2", updated.successes)
	}
	if updated.failures != 1 {
		t.Fatalf("failures = %d, want 1", updated.failures)
	}
}

func TestRunningViewSupportsLogScrolling(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})
	model.activeView = viewRunning
	model.height = 20
	model.width = 100
	model.logs = nil
	for i := 1; i <= 20; i++ {
		model.logs = append(model.logs, "log "+strings.Repeat(string(rune('0'+(i%10))), 1))
	}
	model.scrollLogsToBottom()
	initialScroll := model.logScroll
	if initialScroll <= 0 {
		t.Fatalf("initial logScroll = %d, want > 0", initialScroll)
	}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated := next.(Model)
	if updated.logScroll >= initialScroll {
		t.Fatalf("logScroll after up = %d, want < %d", updated.logScroll, initialScroll)
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyHome})
	updated = next.(Model)
	if updated.logScroll != 0 {
		t.Fatalf("logScroll after home = %d, want 0", updated.logScroll)
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnd})
	updated = next.(Model)
	if updated.logScroll != initialScroll {
		t.Fatalf("logScroll after end = %d, want %d", updated.logScroll, initialScroll)
	}
}

func TestRunningViewRendersStreamingLogsAndShiftStats(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})
	model.activeView = viewRunning
	model.width = 120
	model.height = 24
	model.currentStage = "execute"
	model.currentFile = "/tmp/case-a.ts"
	model.progressCurrent = 7
	model.progressTotal = 12
	model.successes = 5
	model.failures = 2
	model.logs = []string{
		"[成功] case-a.ts",
		"coco> wrote target file",
		"coco> shift completed",
	}

	view := model.View()

	if !strings.Contains(view, "Planned 12") {
		t.Fatal("running view missing planned count")
	}
	if !strings.Contains(view, "Shifted 5") {
		t.Fatal("running view missing shifted count")
	}
	if !strings.Contains(view, "Failed 2") {
		t.Fatal("running view missing failed count")
	}
	if !strings.Contains(view, "[成功] case-a.ts") || !strings.Contains(view, "coco> shift completed") {
		t.Fatal("running view missing streamed log lines")
	}
	if strings.Contains(view, "日志 3 行") {
		t.Fatal("running view should not render legacy log box title")
	}
	if strings.Contains(view, "┌") || strings.Contains(view, "┐") {
		t.Fatal("running view should not render bordered log box")
	}
}

func TestRunningViewRendersAnalyzeProgressAndCurrentFile(t *testing.T) {
	model := NewModel(config.DefaultConfig(), NewNoopRuntime(), Options{})
	model.activeView = viewRunning
	model.width = 100
	model.height = 20

	next, _ := model.Update(runtimeEventMsg{event: WorkflowEvent{
		Stage:       "analyze",
		Message:     "分析目录结构",
		Current:     2,
		Total:       5,
		CurrentFile: "/tmp/source/case-b.spec.ts",
	}})
	updated := next.(Model)

	view := updated.View()
	if !strings.Contains(view, "2/5") {
		t.Fatal("running view missing analyze progress summary")
	}
	if !strings.Contains(view, "/tmp/source/case-b.spec.ts") {
		t.Fatal("running view missing analyze current file")
	}
	if !strings.Contains(view, "ANALYZE") {
		t.Fatal("running view missing analyze stage label")
	}
}

func assertVisibleDirectories(t *testing.T, dirList list.Model, want []string) {
	t.Helper()
	items := dirList.VisibleItems()
	if len(items) != len(want) {
		t.Fatalf("visible items = %d, want %d", len(items), len(want))
	}
	for i, item := range items {
		got, ok := item.(directoryItem)
		if !ok {
			t.Fatalf("visible item %d has type %T, want directoryItem", i, item)
		}
		if string(got) != want[i] {
			t.Fatalf("visible item %d = %q, want %q", i, got, want[i])
		}
	}
}

func keyMsg(value string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}

type countingRuntime struct {
	analyzeCalls int
	startCalls   int
	dirs         []string
}

func (r *countingRuntime) ListDirectories(root string) ([]string, error) {
	if len(r.dirs) > 0 {
		return r.dirs, nil
	}
	return []string{root}, nil
}

func (r *countingRuntime) ListImmediateDirectories(path string) ([]string, error) {
	return nil, nil
}

func (r *countingRuntime) RunAnalyze(ctx context.Context, dir string, notify func(WorkflowEvent)) (*WorkflowResult, error) {
	r.analyzeCalls++
	notify(WorkflowEvent{Stage: "analyze", Message: "counting analyze"})
	return &WorkflowResult{Mode: "analyze", Dir: dir}, nil
}

func (r *countingRuntime) RunStart(ctx context.Context, dir string, targetBaseDir string, notify func(WorkflowEvent)) (*WorkflowResult, error) {
	r.startCalls++
	notify(WorkflowEvent{Stage: "start", Message: "counting start"})
	return &WorkflowResult{Mode: "start", Dir: dir}, nil
}

func (r *countingRuntime) LoadState(dir string) (executor.StateSnapshot, error) {
	return executor.StateSnapshot{}, nil
}
