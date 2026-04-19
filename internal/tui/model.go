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
	input.Prompt = " > "
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
		logs:                 nil,
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
		prevStage := m.currentStage
		m.currentStage = msg.event.Stage
		if msg.event.Current > 0 {
			m.progressCurrent = msg.event.Current
		}
		if msg.event.Total > 0 {
			if msg.event.Current == 0 && (msg.event.Stage != prevStage || msg.event.Total != m.progressTotal) {
				m.progressCurrent = 0
			}
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
	header := m.renderHeader()
	main := m.renderMain()
	commandPanel := m.renderCommandPanel()
	return strings.Join([]string{header, main, "", commandPanel}, "\n")
}

func (m Model) renderHeader() string {
	contentWidth := maxInt(40, m.width-2)

	// Brand
	brand := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1).
		Render(" nep2midsence ")

	// Breadcrumb: show active view
	viewLabel := string(m.activeView)
	if m.activeView == viewHome {
		viewLabel = "home"
	}
	breadcrumb := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render("  " + viewLabel)

	// Right side: version badge
	versionBadge := ""
	if m.opts.Version != "" {
		versionBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Render("v" + m.opts.Version)
	}

	left := brand + breadcrumb
	headerLine := joinEdge(left, versionBadge, contentWidth)

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Render(strings.Repeat("━", contentWidth))

	return headerLine + "\n" + divider
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
// It always starts from the parent directory of the current working directory.
func (m Model) openTargetDirectoryPicker() (tea.Model, tea.Cmd) {
	startPath := strings.TrimSpace(m.opts.WorkDir)
	if startPath == "" {
		startPath = "."
	}
	startPath = filepath.Dir(filepath.Clean(startPath))
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

	// Stage badge with colored background
	stageName := strings.ToUpper(emptyFallback(m.currentStage, "queued"))
	stageColors := map[string][2]string{
		"QUEUED":      {"15", "243"},
		"PREFLIGHT":   {"15", "208"},
		"SCAN":        {"15", "214"},
		"ANALYZE":     {"15", "33"},
		"GENERATE":    {"15", "141"},
		"EXECUTE":     {"15", "35"},
		"VERIFY":      {"15", "81"},
		"NEP-FIX":     {"15", "220"},
		"COMPILE-FIX": {"15", "208"},
		"COMPLETE":    {"15", "35"},
	}
	fg, bg := "15", "62"
	if colors, ok := stageColors[stageName]; ok {
		fg, bg = colors[0], colors[1]
	}
	stageBadge := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(fg)).
		Background(lipgloss.Color(bg)).
		Padding(0, 1).
		Render(stageName)

	currentFile := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Render(emptyFallback(m.currentFile, "-"))

	var progressIndicator string
	if m.currentStage == "scan" {
		progressIndicator = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true).
			Render("⟳ scanning…")
	} else {
		progressIndicator = renderProgressBar(m.progressCurrent, m.progressTotal, contentWidth/3)
	}

	header := "Running " + stageBadge + "  " + currentFile
	headerLine := joinEdge(header, progressIndicator, contentWidth)

	// Log area
	logLines := m.visibleLogs()
	styledLines := make([]string, len(logLines))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	skipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	for i, line := range logLines {
		switch {
		case strings.HasPrefix(line, "[成功]") || strings.HasPrefix(line, "[已迁移]") || strings.Contains(line, "success"):
			styledLines[i] = successStyle.Render(line)
		case strings.HasPrefix(line, "[失败]") || strings.Contains(line, "failed") || strings.Contains(line, "error"):
			styledLines[i] = failStyle.Render(line)
		case strings.HasPrefix(line, "[跳过]"):
			styledLines[i] = skipStyle.Render(line)
		default:
			styledLines[i] = dimStyle.Render(line)
		}
	}
	logs := strings.Join(styledLines, "\n")
	if strings.TrimSpace(logs) == "" {
		logs = dimStyle.Render("暂无日志")
	}

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Scroll: Up/Down PgUp/PgDn Home/End")
	stats := renderShiftStats(m.progressTotal, m.successes, m.failures)
	footer := joinEdge(hint, stats, contentWidth)

	thinDiv := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("─", contentWidth))

	return strings.Join([]string{headerLine, thinDiv, logs, thinDiv, footer}, "\n")
}

func (m Model) renderMain() string {
	switch m.activeView {
	case viewHelp:
		return m.renderHelpView()
	case viewDirectory:
		return fmt.Sprintf("%s\n\n%s", m.directorySearchInput.View(), m.directoryList.View())
	case viewTargetDirectory:
		breadcrumb := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(m.targetBrowsePath)
		hint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Enter: 进入目录 | Tab: 确认选择 | Esc: 返回上级")
		return fmt.Sprintf("选择目标仓库根目录\n当前: %s\n%s\n\n%s\n\n%s", breadcrumb, hint, m.directorySearchInput.View(), m.directoryList.View())
	case viewRunning:
		return m.renderRunningView()
	case viewResults:
		return m.renderResultsView()
	case viewStatus:
		return renderStatus(m.statusSnapshot)
	case viewHistory:
		return renderHistory(m.statusSnapshot)
	case viewConfig:
		return renderConfig(m.cfg)
	case viewVersion:
		return m.renderVersionView()
	default:
		return m.renderHomeView()
	}
}

func (m Model) renderHomeView() string {
	if len(m.logs) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		welcome := lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true).
			Render("nep → midscene 自动化迁移引擎")
		hint := dimStyle.Render("输入 /start 开始迁移流程，或输入 /help 查看所有命令。")
		return "\n" + welcome + "\n\n" + hint + "\n"
	}
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	lines := make([]string, len(m.logs))
	for i, line := range m.logs {
		lines[i] = dimStyle.Render(line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHelpView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	commands := []struct{ cmd, desc string }{
		{"/help", "Show help"},
		{"/start", "Select a directory and run the full workflow"},
		{"/analyze", "Select a directory and only analyze"},
		{"/status", "Show the latest persisted run state"},
		{"/history", "Show recent persisted runs"},
		{"/config", "View config; /config set <key> <value>; /config save"},
		{"/model", "Switch the external CLI tool: /model <coco|cc|codex>"},
		{"/version", "Show version metadata"},
		{"/clear", "Clear the session log"},
		{"/quit", "Exit the CLI"},
	}

	lines := []string{
		titleStyle.Render("Available Commands"),
		"",
	}
	for _, c := range commands {
		pad := strings.Repeat(" ", maxInt(0, 14-len(c.cmd)))
		lines = append(lines, "  "+cmdStyle.Render(c.cmd)+pad+descStyle.Render(c.desc))
	}
	lines = append(lines, "", dimStyle.Render("  按任意键返回"))
	return strings.Join(lines, "\n")
}

func (m Model) renderResultsView() string {
	if m.lastResult == nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("No result yet.")
	}
	if m.lastResult.Mode == "analyze" {
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
		return strings.Join([]string{
			titleStyle.Render("Analyze Summary"),
			"",
			"  " + labelStyle.Render("Directory  ") + valueStyle.Render(m.lastResult.Dir),
			"  " + labelStyle.Render("Cases found") + valueStyle.Render(fmt.Sprintf(" %d", len(m.lastResult.Analyses))),
		}, "\n")
	}
	reporter := verify.NewReporter()
	return reporter.FormatText(m.lastResult.Report)
}

func (m Model) renderVersionView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	return strings.Join([]string{
		titleStyle.Render("nep2midsence"),
		"",
		"  " + labelStyle.Render("Version    ") + valueStyle.Render(emptyFallback(m.opts.Version, "dev")),
		"  " + labelStyle.Render("Build Date ") + valueStyle.Render(emptyFallback(m.opts.BuildDate, "unknown")),
		"  " + labelStyle.Render("Git Commit ") + valueStyle.Render(emptyFallback(m.opts.GitCommit, "unknown")),
	}, "\n")
}

func (m Model) renderCommandPanel() string {
	contentWidth := maxInt(20, m.width-2)

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Render(strings.Repeat("━", contentWidth))

	label := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1).
		Render("Command")

	inputLine := label + "  " + m.commandInput.View()

	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	hint := hintStyle.Render("  /help /start /analyze /status /history /config /model /version /clear /quit")

	content := []string{divider, inputLine, hint}

	if m.lastError != "" {
		errStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
		content = append(content, "  "+errStyle.Render("! "+m.lastError))
	}
	if m.lastInfo != "" {
		infoStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("35"))
		content = append(content, "  "+infoStyle.Render(m.lastInfo))
	}

	return strings.Join(content, "\n")
}

func renderConfig(cfg *config.Config) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	lines := []string{titleStyle.Render("Current Config"), ""}
	for _, key := range cfg.GetAllKeys() {
		value, err := cfg.Get(key)
		if err != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("  %s %s %v",
			keyStyle.Render(key),
			dimStyle.Render("="),
			valStyle.Render(fmt.Sprintf("%v", value))))
	}
	return strings.Join(lines, "\n")
}

func renderStatus(snapshot executor.StateSnapshot) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	if snapshot.CurrentRun == nil {
		return dimStyle.Render("暂无可展示的状态。先执行 /start。")
	}
	run := snapshot.CurrentRun

	statusColor := "243"
	switch run.Status {
	case "completed":
		statusColor = "35"
	case "running":
		statusColor = "214"
	case "failed":
		statusColor = "196"
	}
	statusBadge := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color(statusColor)).
		Padding(0, 1).
		Render(run.Status)

	return strings.Join([]string{
		titleStyle.Render("Latest Run"),
		"",
		"  " + labelStyle.Render("ID          ") + valueStyle.Render(run.ID),
		"  " + labelStyle.Render("Directory   ") + valueStyle.Render(run.Dir),
		"  " + labelStyle.Render("Status      ") + statusBadge,
		"  " + labelStyle.Render("Progress    ") + valueStyle.Render(fmt.Sprintf("%d/%d", run.Completed+run.Failed, run.TotalFiles)),
		"  " + labelStyle.Render("Succeeded   ") + lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Bold(true).Render(fmt.Sprintf("%d", run.Completed)),
		"  " + labelStyle.Render("Failed      ") + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf("%d", run.Failed)),
		"  " + labelStyle.Render("Current     ") + valueStyle.Render(emptyFallback(run.CurrentFile, "-")),
	}, "\n")
}

func renderHistory(snapshot executor.StateSnapshot) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	if len(snapshot.Runs) == 0 {
		return dimStyle.Render("暂无历史记录。")
	}
	lines := []string{
		titleStyle.Render("Run History"),
		"",
		dimStyle.Render("  Time                 Dir                  Status    Progress"),
		dimStyle.Render("  " + strings.Repeat("─", 70)),
	}
	for _, run := range snapshot.Runs {
		statusColor := "243"
		switch run.Status {
		case "completed":
			statusColor = "35"
		case "failed":
			statusColor = "196"
		}
		statusRendered := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(run.Status)
		lines = append(lines, "  "+rowStyle.Render(fmt.Sprintf("%-21s %-20s ", run.StartedAt.Format("2006-01-02 15:04:05"), truncateString(run.Dir, 20)))+statusRendered+rowStyle.Render(fmt.Sprintf("  %d/%d", run.Completed+run.Failed, run.TotalFiles)))
	}
	return strings.Join(lines, "\n")
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
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

func renderProgressBar(current, total, barWidth int) string {
	if total <= 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("0/0")
	}
	if barWidth < 8 {
		barWidth = 8
	}

	pct := float64(current) / float64(total)
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	filledStr := strings.Repeat("█", filled)
	emptyStr := strings.Repeat("░", barWidth-filled)

	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Render(filledStr) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(emptyStr)

	counter := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render(fmt.Sprintf(" %d/%d", current, total))

	return bar + counter
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
