package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/executor"
	"github.com/sandaogouchen/nep2midsence/internal/verify"
)

// Options configure the TUI shell.
type Options struct {
	ConfigPath string
	Verbose    bool
	WorkDir    string
	Version    string
	BuildDate  string
	GitCommit  string
}

type viewState string

const (
	viewHome            viewState = "home"
	viewHelp            viewState = "help"
	viewDirectory       viewState = "directory"
	viewTargetDirectory viewState = "target_directory"
	viewRunning         viewState = "running"
	viewResults         viewState = "results"
	viewStatus          viewState = "status"
	viewHistory         viewState = "history"
	viewConfig          viewState = "config"
	viewVersion         viewState = "version"
)

type workflowMode string

const (
	modeStart   workflowMode = "start"
	modeAnalyze workflowMode = "analyze"
)

type submitCommandMsg struct{}
type runtimeEventMsg struct{ event WorkflowEvent }
type runtimeFinishedMsg struct {
	result *WorkflowResult
	err    error
}

type directoryItem string

func (d directoryItem) FilterValue() string { return string(d) }
func (d directoryItem) Title() string       { return string(d) }
func (d directoryItem) Description() string { return "directory" }

// Model is the full-screen Bubble Tea application.
type Model struct {
	cfg                  *config.Config
	runtime              Runtime
	opts                 Options
	activeView           viewState
	commandInput         textinput.Model
	directoryList        list.Model
	directorySearchInput textinput.Model
	allDirectories       []string
	pickerMode           workflowMode
	lastError            string
	lastInfo             string
	logs                 []string
	quitting             bool
	width                int
	height               int
	runtimeMsgs          chan tea.Msg
	currentDir           string
	currentStage         string
	progressCurrent      int
	progressTotal        int
	successes            int
	failures             int
	currentFile          string
	lastResult           *WorkflowResult
	statusSnapshot       executor.StateSnapshot
	logScroll            int
	targetBaseDir        string // cross-repo target directory
	targetBrowsePath     string // current path in target directory browser
}

func NewModel(cfg *config.Config, runtime Runtime, opts Options) Model {
	input := textinput.New()
	input.Placeholder = "/help"
	input.Prompt = " / "
	input.Focus()
	input.CharLimit = 512
	input.Width = 80

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	dirList := list.New([]list.Item{}, delegate, 0, 0)
	dirList.Title = "选择目录"
	dirList.SetFilteringEnabled(false)
	dirList.SetShowStatusBar(false)
	dirList.SetShowHelp(false)

	dirSearch := textinput.New()
	dirSearch.Placeholder = "输入关键字筛选目录"
	dirSearch.Prompt = " Search> "
	dirSearch.CharLimit = 256
	dirSearch.Width = 80

	if opts.WorkDir == "" {
		opts.WorkDir = "."
	}

	return Model{
		cfg:                  cfg,
		runtime:              runtime,
		opts:                 opts,
		activeView:           viewHome,
		commandInput:         input,
		directoryList:        dirList,
		directorySearchInput: dirSearch,
		logs:                 []string{"欢迎使用 nep2midsence 全屏交互模式。"},
		currentDir:           opts.WorkDir,
	}
}

func NewProgram(cfg *config.Config, runtime Runtime, opts Options) *tea.Program {
	return tea.NewProgram(NewModel(cfg, runtime, opts), tea.WithAltScreen())
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.directoryList.SetSize(msg.Width-6, msg.Height-13)
		m.directorySearchInput.Width = maxInt(20, msg.Width-12)
		return m, nil
	case submitCommandMsg:
		return m.handleCommand()
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}
		if m.activeView == viewDirectory {
			switch msg.String() {
			case "esc":
				m.activeView = viewHome
				return m, nil
			case "enter":
				if item, ok := m.directoryList.SelectedItem().(directoryItem); ok {
					selected := string(item)
					if m.pickerMode == modeStart {
						// Phase 1 complete: source dir selected, transition to target dir picker.
						m.currentDir = selected
						return m.openTargetDirectoryPicker()
					}
					return m.beginWorkflow(selected)
				}
			case "up", "down", "left", "right", "pgup", "pgdown", "home", "end":
				var cmd tea.Cmd
				m.directoryList, cmd = m.directoryList.Update(msg)
				return m, cmd
			}
			var cmd tea.Cmd
			m.directorySearchInput, cmd = m.directorySearchInput.Update(msg)
			m.applyDirectoryFilter(m.directorySearchInput.Value())
			return m, cmd
		}
		if m.activeView == viewTargetDirectory {
			switch msg.String() {
			case "esc":
				// Go up one level; if at root, return to source picker.
				parent := filepath.Dir(m.targetBrowsePath)
				if parent == m.targetBrowsePath {
					// Already at filesystem root, go back to source directory picker.
					return m.openDirectoryPicker(modeStart)
				}
				m.targetBrowsePath = parent
				return m.refreshTargetDirectoryList()
			case "enter":
				if item, ok := m.directoryList.SelectedItem().(directoryItem); ok {
					// Navigate into the selected subdirectory.
					m.targetBrowsePath = string(item)
					return m.refreshTargetDirectoryList()
				}
			case "tab":
				// Confirm current browse path as target directory.
				m.targetBaseDir = m.targetBrowsePath
				return m.beginWorkflow(m.currentDir)
			case "up", "down", "left", "right", "pgup", "pgdown", "home", "end":
				var cmd tea.Cmd
				m.directoryList, cmd = m.directoryList.Update(msg)
				return m, cmd
			}
			var cmd tea.Cmd
			m.directorySearchInput, cmd = m.directorySearchInput.Update(msg)
			m.applyDirectoryFilter(m.directorySearchInput.Value())
			return m, cmd
		}
		if m.activeView == viewRunning {
			switch msg.String() {
			case "up":
				m.scrollLogsBy(-1)
				return m, nil
			case "down":
				m.scrollLogsBy(1)
				return m, nil
			case "pgup":
				m.scrollLogsBy(-m.logViewportHeight())
				return m, nil
			case "pgdown":
				m.scrollLogsBy(m.logViewportHeight())
				return m, nil
			case "home":
				m.logScroll = 0
				return m, nil
			case "end":
				m.scrollLogsToBottom()
				return m, nil
			}
		}

		if msg.String() == "enter" {
			return m, func() tea.Msg { return submitCommandMsg{} }
		}
		var cmd tea.Cmd
		m.commandInput, cmd = m.commandInput.Update(msg)
		return m, cmd
	case runtimeEventMsg:
		m.currentStage = msg.event.Stage
		if msg.event.Current > 0 {
			m.progressCurrent = msg.event.Current
		}
		if msg.event.Total > 0 {
			m.progressTotal = msg.event.Total
		}
		if msg.event.Successes > 0 || (msg.event.Current == 0 && msg.event.Total == 0 && msg.event.Failures > 0) {
			m.successes = msg.event.Successes
		}
		if msg.event.Failures > 0 || (msg.event.Current == 0 && msg.event.Total == 0 && msg.event.Successes > 0) {
			m.failures = msg.event.Failures
		}
		if msg.event.CurrentFile != "" {
			m.currentFile = msg.event.CurrentFile
		}
		if msg.event.Message != "" {
			m.appendLog(msg.event.Message)
		}
		if len(msg.event.LogLines) > 0 {
			m.appendLogLines(msg.event.LogLines)
		}
		if m.runtimeMsgs != nil {
			return m, waitForRuntimeMsg(m.runtimeMsgs)
		}
		return m, nil
	case runtimeFinishedMsg:
		m.runtimeMsgs = nil
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.activeView = viewRunning
			return m, nil
		}
		m.lastResult = msg.result
		m.activeView = viewResults
		if msg.result != nil && msg.result.Dir != "" {
			m.currentDir = msg.result.Dir
		}
		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("nep2midsence")
	main := m.renderMain()
	commandPanel := m.renderCommandPanel()
	return strings.Join([]string{header, "", main, "", commandPanel}, "\n")
}

func (m Model) handleCommand() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(m.commandInput.Value())
	m.commandInput.SetValue("")
	m.lastInfo = ""

	if line == "" {
		return m, nil
	}
	if !strings.HasPrefix(line, "/") {
		m.lastError = "仅支持命令输入，使用 /help 查看可用命令"
		return m, nil
	}

	parts := strings.Fields(strings.TrimPrefix(line, "/"))
	if len(parts) == 0 {
		return m, nil
	}

	m.lastError = ""

	switch parts[0] {
	case "help":
		m.activeView = viewHelp
		return m, nil
	case "clear":
		m.logs = nil
		m.logScroll = 0
		m.lastError = ""
		m.lastInfo = ""
		return m, nil
	case "quit", "exit":
		m.quitting = true
		return m, tea.Quit
	case "version":
		m.activeView = viewVersion
		return m, nil
	case "config":
		return m.handleConfig(parts[1:])
	case "model":
		return m.handleModel(parts[1:])
	case "start":
		return m.openDirectoryPicker(modeStart)
	case "analyze":
		return m.openDirectoryPicker(modeAnalyze)
	case "status":
		snapshot, err := m.runtime.LoadState(m.currentDir)
		if err != nil {
			m.lastError = err.Error()
			return m, nil
		}
		m.statusSnapshot = snapshot
		m.activeView = viewStatus
		return m, nil
	case "history":
		snapshot, err := m.runtime.LoadState(m.currentDir)
		if err != nil {
			m.lastError = err.Error()
			return m, nil
		}
		m.statusSnapshot = snapshot
		m.activeView = viewHistory
		return m, nil
	default:
		m.lastError = fmt.Sprintf("未知命令: /%s", parts[0])
		return m, nil
	}
}

func (m Model) handleModel(args []string) (tea.Model, tea.Cmd) {
	current := m.cfg.Execution.Tool
	if strings.TrimSpace(current) == "" {
		current = executor.ToolCoco
	}
	current = executor.NormalizeToolForConfig(current)

	if len(args) == 0 {
		m.lastInfo = fmt.Sprintf("当前 CLI 工具: %s；用法: /model <coco|cc|codex>", current)
		return m, nil
	}

	selected := executor.NormalizeToolForConfig(args[0])
	switch selected {
	case executor.ToolCoco, executor.ToolCC, executor.ToolCodex:
		m.cfg.Execution.Tool = selected
		m.lastInfo = fmt.Sprintf("已切换 CLI 工具为: %s", selected)
		return m, nil
	default:
		m.lastError = fmt.Sprintf("不支持的 CLI 工具: %s (可选: coco, cc, codex)", args[0])
		return m, nil
	}
}

func (m Model) handleConfig(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.activeView = viewConfig
		return m, nil
	}

	switch args[0] {
	case "set":
		if len(args) < 3 {
			m.lastError = "用法: /config set <key> <value>"
			return m, nil
		}
		if err := m.cfg.Set(args[1], strings.Join(args[2:], " ")); err != nil {
			m.lastError = err.Error()
			return m, nil
		}
		m.lastInfo = fmt.Sprintf("已设置 %s", args[1])
		m.activeView = viewConfig
		return m, nil
	case "save":
		if err := m.cfg.Save(); err != nil {
			m.lastError = err.Error()
			return m, nil
		}
		m.lastInfo = "配置已保存"
		m.activeView = viewConfig
		return m, nil
	default:
		m.lastError = "支持的 /config 子命令: set, save"
		return m, nil
	}
}

func (m Model) openDirectoryPicker(mode workflowMode) (tea.Model, tea.Cmd) {
	dirs, err := m.runtime.ListDirectories(m.opts.WorkDir)
	if err != nil {
		m.lastError = err.Error()
		return m, nil
	}
	m.allDirectories = append([]string(nil), dirs...)
	m.directorySearchInput.SetValue("")
	cmd := m.directorySearchInput.Focus()
	m.applyDirectoryFilter("")
	m.pickerMode = mode
	m.activeView = viewDirectory
	m.targetBaseDir = ""
	m.lastInfo = "输入关键字搜索目录，方向键选择，回车确认，Esc 返回"
	return m, cmd
}

// openTargetDirectoryPicker transitions to the cross-repo target directory browser.
// It starts from the previously saved target.base_dir (if any) or the filesystem root.
func (m Model) openTargetDirectoryPicker() (tea.Model, tea.Cmd) {
	startPath := strings.TrimSpace(m.cfg.Target.BaseDir)
	if startPath == "" {
		startPath = "/"
	}
	m.targetBrowsePath = startPath
	m.activeView = viewTargetDirectory
	m.lastInfo = "浏览并选择目标仓库目录：Enter 进入子目录，Tab 确认当前目录，Esc 返回上级"
	return m.refreshTargetDirectoryList()
}

// refreshTargetDirectoryList reloads the immediate subdirectories for the current
// browse path in the target directory picker.
func (m Model) refreshTargetDirectoryList() (tea.Model, tea.Cmd) {
	dirs, err := m.runtime.ListImmediateDirectories(m.targetBrowsePath)
	if err != nil {
		m.lastError = fmt.Sprintf("读取目录失败: %s", err)
		return m, nil
	}
	m.allDirectories = dirs
	m.directorySearchInput.SetValue("")
	cmd := m.directorySearchInput.Focus()
	m.applyDirectoryFilter("")
	m.directoryList.Title = fmt.Sprintf("目标目录: %s", m.targetBrowsePath)
	return m, cmd
}

func (m Model) beginWorkflow(dir string) (tea.Model, tea.Cmd) {
	m.activeView = viewRunning
	m.currentDir = dir
	m.currentStage = "queued"
	m.progressCurrent = 0
	m.progressTotal = 0
	m.successes = 0
	m.failures = 0
	m.currentFile = ""
	m.lastResult = nil
	m.appendLog(fmt.Sprintf("已选择目录: %s", dir))
	if m.targetBaseDir != "" {
		m.appendLog(fmt.Sprintf("跨仓库目标: %s", m.targetBaseDir))
	}

	ch := make(chan tea.Msg, 32)
	m.runtimeMsgs = ch

	mode := m.pickerMode
	runtime := m.runtime
	targetBaseDir := m.targetBaseDir
	go func() {
		notify := func(event WorkflowEvent) {
			ch <- runtimeEventMsg{event: event}
		}
		ctx := context.Background()
		var result *WorkflowResult
		var err error
		if mode == modeAnalyze {
			result, err = runtime.RunAnalyze(ctx, dir, notify)
		} else {
			result, err = runtime.RunStart(ctx, dir, targetBaseDir, notify)
		}
		ch <- runtimeFinishedMsg{result: result, err: err}
		close(ch)
	}()

	return m, waitForRuntimeMsg(ch)
}

func waitForRuntimeMsg(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *Model) appendLog(line string) {
	m.appendLogLines([]string{line})
}

func (m *Model) appendLogLines(lines []string) {
	if len(lines) == 0 {
		return
	}

	atBottom := m.logScroll >= m.maxLogScroll()
	for _, line := range lines {
		normalized := strings.ReplaceAll(line, "\r\n", "\n")
		for _, part := range strings.Split(normalized, "\n") {
			m.logs = append(m.logs, part)
		}
	}
	if atBottom {
		m.scrollLogsToBottom()
		return
	}
	m.clampLogScroll()
}

func (m *Model) scrollLogsBy(delta int) {
	m.logScroll += delta
	m.clampLogScroll()
}

func (m *Model) scrollLogsToBottom() {
	m.logScroll = m.maxLogScroll()
}

func (m *Model) clampLogScroll() {
	if m.logScroll < 0 {
		m.logScroll = 0
	}
	maxScroll := m.maxLogScroll()
	if m.logScroll > maxScroll {
		m.logScroll = maxScroll
	}
}

func (m Model) maxLogScroll() int {
	visible := m.logViewportHeight()
	if len(m.logs) <= visible {
		return 0
	}
	return len(m.logs) - visible
}

func (m Model) logViewportHeight() int {
	if m.height <= 0 {
		return 8
	}
	if m.activeView == viewRunning {
		return maxInt(6, m.height-11)
	}
	return maxInt(6, m.height-16)
}

func (m Model) visibleLogs() []string {
	if len(m.logs) == 0 {
		return []string{"暂无日志"}
	}

	height := m.logViewportHeight()
	start := m.logScroll
	if start > len(m.logs) {
		start = len(m.logs)
	}
	end := minInt(start+height, len(m.logs))
	return append([]string(nil), m.logs[start:end]...)
}

func (m Model) renderRunningView() string {
	contentWidth := maxInt(40, m.width-2)
	stage := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")).Render(strings.ToUpper(emptyFallback(m.currentStage, "queued")))
	current := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(emptyFallback(m.currentFile, "-"))
	header := joinEdge("Running "+stage+"  "+current, renderProgressSummary(m.progressCurrent, m.progressTotal), contentWidth)

	logs := strings.Join(m.visibleLogs(), "\n")
	if strings.TrimSpace(logs) == "" {
		logs = "暂无日志"
	}

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Scroll: Up/Down PgUp/PgDn Home/End")
	stats := renderShiftStats(m.progressTotal, m.successes, m.failures)
	footer := joinEdge(hint, stats, contentWidth)
	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("─", contentWidth))

	return strings.Join([]string{header, divider, logs, divider, footer}, "\n")
}

func (m Model) renderMain() string {
	switch m.activeView {
	case viewHelp:
		return strings.Join([]string{
			"Available Commands",
			"/help      Show help",
			"/start     Select a directory and run the full workflow",
			"/analyze   Select a directory and only analyze",
			"/status    Show the latest persisted run state",
			"/history   Show recent persisted runs",
			"/config    View config; /config set <key> <value>; /config save",
			"/model     Switch the external CLI tool: /model <coco|cc|codex>",
			"/version   Show version metadata",
			"/clear     Clear the session log",
			"/quit      Exit the CLI",
		}, "\n")
	case viewDirectory:
		return fmt.Sprintf("%s\n\n%s", m.directorySearchInput.View(), m.directoryList.View())
	case viewTargetDirectory:
		breadcrumb := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(m.targetBrowsePath)
		hint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Enter: 进入目录 | Tab: 确认选择 | Esc: 返回上级")
		return fmt.Sprintf("选择目标仓库根目录\n当前: %s\n%s\n\n%s\n\n%s", breadcrumb, hint, m.directorySearchInput.View(), m.directoryList.View())
	case viewRunning:
		return m.renderRunningView()
	case viewResults:
		if m.lastResult == nil {
			return "No result yet."
		}
		if m.lastResult.Mode == "analyze" {
			return fmt.Sprintf("Analyze Summary\nDirectory: %s\nCases found: %d", m.lastResult.Dir, len(m.lastResult.Analyses))
		}
		reporter := verify.NewReporter()
		return reporter.FormatText(m.lastResult.Report)
	case viewStatus:
		return renderStatus(m.statusSnapshot)
	case viewHistory:
		return renderHistory(m.statusSnapshot)
	case viewConfig:
		return renderConfig(m.cfg)
	case viewVersion:
		return fmt.Sprintf("nep2midsence %s\nBuild Date: %s\nGit Commit: %s", m.opts.Version, m.opts.BuildDate, m.opts.GitCommit)
	default:
		return strings.Join(m.logs, "\n")
	}
}

func (m Model) renderCommandPanel() string {
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69")).Render("Command")
	content := []string{
		hintStyle.Render(strings.Repeat("─", maxInt(20, m.width-2))),
		label + "  " + m.commandInput.View(),
		hintStyle.Render("Slash commands: /help /start /analyze /status /history /config /model /version /clear /quit"),
	}
	if m.lastError != "" {
		content = append(content, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.lastError))
	}
	if m.lastInfo != "" {
		content = append(content, lipgloss.NewStyle().Foreground(lipgloss.Color("70")).Render(m.lastInfo))
	}

	return strings.Join(content, "\n")
}

func renderConfig(cfg *config.Config) string {
	lines := []string{"Current Config"}
	for _, key := range cfg.GetAllKeys() {
		value, err := cfg.Get(key)
		if err != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s = %v", key, value))
	}
	return strings.Join(lines, "\n")
}

func renderStatus(snapshot executor.StateSnapshot) string {
	if snapshot.CurrentRun == nil {
		return "暂无可展示的状态。先执行 /start。"
	}
	run := snapshot.CurrentRun
	return fmt.Sprintf("Latest Run\nID: %s\nDir: %s\nStatus: %s\nProgress: %d/%d\nSucceeded: %d\nFailed: %d\nCurrent file: %s",
		run.ID, run.Dir, run.Status, run.Completed+run.Failed, run.TotalFiles, run.Completed, run.Failed, emptyFallback(run.CurrentFile, "-"))
}

func renderHistory(snapshot executor.StateSnapshot) string {
	if len(snapshot.Runs) == 0 {
		return "暂无历史记录。"
	}
	lines := []string{"Run History"}
	for _, run := range snapshot.Runs {
		lines = append(lines, fmt.Sprintf("%s  %s  %s  %d/%d", run.StartedAt.Format("2006-01-02 15:04:05"), run.Dir, run.Status, run.Completed+run.Failed, run.TotalFiles))
	}
	return strings.Join(lines, "\n")
}

func emptyFallback(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func renderProgressSummary(current, total int) string {
	if total <= 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("0/0")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render(fmt.Sprintf("%d/%d", current, total))
}

func renderShiftStats(planned, shifted, failed int) string {
	pill := func(label string, value int, color lipgloss.Color) string {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(color).
			Bold(true).
			Padding(0, 1).
			Render(fmt.Sprintf("%s %d", label, value))
	}

	return strings.Join([]string{
		pill("Planned", planned, lipgloss.Color("62")),
		pill("Shifted", shifted, lipgloss.Color("35")),
		pill("Failed", failed, lipgloss.Color("160")),
	}, " ")
}

func joinEdge(left, right string, width int) string {
	if right == "" {
		return left
	}
	if left == "" {
		return right
	}
	if width <= 0 {
		return left + " " + right
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) applyDirectoryFilter(query string) {
	filtered := make([]list.Item, 0, len(m.allDirectories))
	for _, dir := range m.allDirectories {
		if matchesDirectoryQuery(dir, query) {
			filtered = append(filtered, directoryItem(dir))
		}
	}
	m.directoryList.SetItems(filtered)
	m.directoryList.ResetSelected()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func matchesDirectoryQuery(dir, query string) bool {
	tokens := tokenizeDirectoryQuery(query)
	if len(tokens) == 0 {
		return true
	}

	parts := normalizeDirectoryParts(dir)
	partIndex := 0
	charIndex := 0

	for _, token := range tokens {
		found := false
		for i := partIndex; i < len(parts); i++ {
			start := 0
			if i == partIndex {
				start = charIndex
			}
			if start > len(parts[i]) {
				start = len(parts[i])
			}
			pos := strings.Index(parts[i][start:], token)
			if pos >= 0 {
				partIndex = i
				charIndex = start + pos + len(token)
				found = true
				break
			}
			charIndex = 0
		}
		if !found {
			return false
		}
	}
	return true
}

func tokenizeDirectoryQuery(query string) []string {
	rawTokens := strings.FieldsFunc(strings.TrimSpace(query), func(r rune) bool {
		return unicode.IsSpace(r) || r == '/' || r == '\\'
	})
	tokens := make([]string, 0, len(rawTokens))
	for _, token := range rawTokens {
		normalized := normalizeSearchToken(token)
		if normalized != "" {
			tokens = append(tokens, normalized)
		}
	}
	return tokens
}

func normalizeDirectoryParts(dir string) []string {
	rawParts := strings.FieldsFunc(dir, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		normalized := normalizeSearchToken(part)
		if normalized != "" {
			parts = append(parts, normalized)
		}
	}
	return parts
}

func normalizeSearchToken(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
