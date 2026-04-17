package tui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/analyzer"
	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/executor"
	"github.com/sandaogouchen/nep2midsence/internal/prompt"
	"github.com/sandaogouchen/nep2midsence/internal/types"
	"github.com/sandaogouchen/nep2midsence/internal/verify"
)

// WorkflowEvent streams progress updates into the TUI.
type WorkflowEvent struct {
	Stage       string
	Message     string
	Current     int
	Total       int
	Successes   int
	Failures    int
	CurrentFile string
	LogLines    []string
}

// WorkflowResult captures the final outcome of an analyze/start command.
type WorkflowResult struct {
	Mode     string
	Dir      string
	Analyses []*types.FullAnalysis
	Results  []*types.MigrationResult
	Verifies []*types.VerifyResult
	Report   *types.MigrationReport
}

// Runtime is the execution surface the TUI needs.
type Runtime interface {
	ListDirectories(root string) ([]string, error)
	ListImmediateDirectories(path string) ([]string, error)
	RunAnalyze(ctx context.Context, dir string, notify func(WorkflowEvent)) (*WorkflowResult, error)
	RunStart(ctx context.Context, dir string, targetBaseDir string, notify func(WorkflowEvent)) (*WorkflowResult, error)
	LoadState(dir string) (executor.StateSnapshot, error)
}

// AppRuntime is the concrete runtime wired to the existing analyzer/executor/verify pipeline.
type AppRuntime struct {
	cfg *config.Config
}

func NewRuntime(cfg *config.Config) *AppRuntime {
	return &AppRuntime{cfg: cfg}
}

func (r *AppRuntime) ListDirectories(root string) ([]string, error) {
	seen := map[string]struct{}{}
	dirs := []string{root}
	seen[root] = struct{}{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".git" || name == "node_modules" || name == "dist" {
			return filepath.SkipDir
		}
		if _, ok := seen[path]; ok {
			return nil
		}
		seen[path] = struct{}{}
		dirs = append(dirs, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(dirs)
	return dirs, nil
}

// ListImmediateDirectories returns one level of child directories for the
// given path, suitable for breadcrumb-style directory browsing in cross-repo
// target selection. It skips .git, node_modules, and dist directories.
func (r *AppRuntime) ListImmediateDirectories(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == ".git" || name == "node_modules" || name == "dist" {
			continue
		}
		dirs = append(dirs, filepath.Join(path, name))
	}
	sort.Strings(dirs)
	return dirs, nil
}

func (r *AppRuntime) RunAnalyze(ctx context.Context, dir string, notify func(WorkflowEvent)) (*WorkflowResult, error) {
	notify(WorkflowEvent{Stage: "analyze", Message: "正在分析目录", CurrentFile: dir})
	engine := analyzer.NewEngine(r.cfg)
	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		return nil, err
	}

	notify(WorkflowEvent{Stage: "analyze", Message: fmt.Sprintf("分析完成，共 %d 个 case", len(analyses)), Total: len(analyses)})
	return &WorkflowResult{
		Mode:     "analyze",
		Dir:      dir,
		Analyses: analyses,
	}, nil
}

func (r *AppRuntime) RunStart(ctx context.Context, dir string, targetBaseDir string, notify func(WorkflowEvent)) (*WorkflowResult, error) {
	tool := executor.NormalizeToolForConfig(r.cfg.Execution.Tool)
	if tool == "" {
		tool = executor.ToolCoco
	}
	notify(WorkflowEvent{Stage: "preflight", Message: fmt.Sprintf("检查 %s", tool)})
	if err := executor.PreflightCheckForTool(tool); err != nil {
		return nil, err
	}

	engine := analyzer.NewEngine(r.cfg)

	// Cross-repo mode: set target base dir in config and detect source repo root.
	if strings.TrimSpace(targetBaseDir) != "" {
		r.cfg.Target.BaseDir = targetBaseDir
		sourceRoot := analyzer.DetectSourceRepoRoot(dir)
		engine.SetSourceRepoRoot(sourceRoot)
	}

	notify(WorkflowEvent{Stage: "analyze", Message: "分析目录结构"})
	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		return nil, err
	}

	// Store state in project root (where .nep2midsence.yaml lives), not in the selected directory.
	store, err := executor.NewStateStore(r.cfg.ConfigDir())
	if err != nil {
		return nil, err
	}

	// Build execution plan: migrate shared helper/wrapper files first, then migrate cases.
	plan, err := buildMigrationPlan(engine, analyses, store)
	if err != nil {
		return nil, err
	}

	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	startedAt := time.Now()
	if err := store.StartRun(runID, dir, targetBaseDir, plan.TotalPlanned, startedAt); err != nil {
		return nil, err
	}

	notify(WorkflowEvent{Stage: "generate", Message: fmt.Sprintf("生成迁移计划，共 %d 个任务（含封装/helper）", plan.TotalPlanned), Total: plan.TotalPlanned})
	gen := prompt.NewGenerator(r.cfg)

	// Determine executor work directory: use targetBaseDir for cross-repo mode.
	execWorkDir := dir
	if strings.TrimSpace(targetBaseDir) != "" {
		execWorkDir = targetBaseDir
		// Ensure target base directory exists.
		if err := os.MkdirAll(targetBaseDir, 0o755); err != nil {
			return nil, fmt.Errorf("create target base dir: %w", err)
		}
	}

	var promptExec executor.PromptExecutor
	switch tool {
	case executor.ToolCC:
		promptExec = executor.NewCCExecutor(execWorkDir, 0)
	case executor.ToolCodex:
		promptExec = executor.NewCodexExecutor(execWorkDir, 0)
	default:
		promptExec = executor.NewCocoExecutor(execWorkDir, 0)
	}

	scheduler := executor.NewScheduler(promptExec, gen, r.cfg.Execution.MaxJobs, r.cfg.Execution.RetryLimit)

	// Record skipped tasks immediately (cross-run dedupe), then execute remaining tasks.
	successes := 0
	failures := 0
	completed := 0
	var progressMu sync.Mutex

	for _, t := range plan.Skipped {
		completed++
		successes++
		_ = store.RecordTaskResult(runID, t.File, "skipped", "up-to-date", t.Kind, t.SourceHash, t.TargetFile, time.Now())
		notify(WorkflowEvent{
			Stage:       "execute",
			Current:     completed,
			Total:       plan.TotalPlanned,
			Successes:   successes,
			Failures:    failures,
			CurrentFile: t.File,
			LogLines:    []string{fmt.Sprintf("[跳过] %s", t.File)},
		})
	}

	for _, t := range plan.AlreadyMigrated {
		notify(WorkflowEvent{
			Stage:       "execute",
			CurrentFile: t.File,
			LogLines:    []string{fmt.Sprintf("[已迁移] %s -> %s (Midscene 框架已检测到，跳过)", t.File, t.TargetFile)},
		})
	}

	scheduler.SetProgressCallback(func(result *types.MigrationResult, _current, _total int) {
		if result == nil {
			return
		}
		file := result.CaseFile
		meta, ok := plan.MetaByFile[file]
		if !ok {
			meta = planItemMeta{File: file, Kind: "case", TargetFile: result.TargetFile}
		}

		status := "failed"
		errMsg := result.Error
		if result.Success {
			status = "completed"
			errMsg = ""
		}

		progressMu.Lock()
		completed++
		if status == "completed" {
			successes++
		} else {
			failures++
		}
		current := completed
		progressMu.Unlock()

		_ = store.RecordTaskResult(runID, file, status, errMsg, meta.Kind, meta.SourceHash, meta.TargetFile, time.Now())
		notify(WorkflowEvent{
			Stage:       "execute",
			Current:     current,
			Total:       plan.TotalPlanned,
			Successes:   successes,
			Failures:    failures,
			CurrentFile: file,
			LogLines:    buildExecutionLogLines(result, tool),
		})
	})

	notify(WorkflowEvent{Stage: "execute", Message: "开始执行迁移"})
	var results []*types.MigrationResult
	if len(plan.ToExecuteHelpers) > 0 {
		results = append(results, scheduler.Run(ctx, plan.ToExecuteHelpers)...)
	}
	if len(plan.ToExecuteCases) > 0 {
		results = append(results, scheduler.Run(ctx, plan.ToExecuteCases)...)
	}

	notify(WorkflowEvent{Stage: "verify", Message: "验证迁移结果"})
	verifyDir := dir
	if strings.TrimSpace(targetBaseDir) != "" {
		verifyDir = targetBaseDir
	}
	verifier := verify.NewVerifier(verifyDir, "", "")
	verifyResults := verifier.VerifyAll(results)

	// NEP residual fix loop: re-execute AI fix for files that still contain NEP markers.
	maxNepFixRounds := r.cfg.Execution.RetryLimit
	if maxNepFixRounds <= 0 {
		maxNepFixRounds = 2
	}
	for round := 1; round <= maxNepFixRounds; round++ {
		var nepFailedIdx []int
		for i, vr := range verifyResults {
			if !vr.NepCleanOK && vr.NepCleanError != "" && results[i].Success {
				nepFailedIdx = append(nepFailedIdx, i)
			}
		}
		if len(nepFailedIdx) == 0 {
			break
		}

		notify(WorkflowEvent{
			Stage:   "nep-fix",
			Message: fmt.Sprintf("NEP 残留修正（第 %d 轮，%d 个文件）", round, len(nepFailedIdx)),
			Total:   len(nepFailedIdx),
		})

		for fixNum, idx := range nepFailedIdx {
			res := results[idx]
			vr := verifyResults[idx]
			fixPrompt := gen.GenerateNepFixPrompt(res.TargetFile, vr.NepCleanError, round)

			notify(WorkflowEvent{
				Stage:       "nep-fix",
				Message:     fmt.Sprintf("修正 %s", filepath.Base(res.TargetFile)),
				Current:     fixNum + 1,
				Total:       len(nepFailedIdx),
				CurrentFile: res.TargetFile,
			})

			output, execErr := promptExec.Execute(ctx, fixPrompt)
			res.NepFixAttempts = round
			if execErr == nil && output.Success {
				// Re-verify NEP cleanliness after fix.
				cleanOK, cleanErr := verify.CheckNepClean(res.TargetFile)
				verifyResults[idx].NepCleanOK = cleanOK
				verifyResults[idx].NepCleanError = cleanErr
			}
		}
	}

	reporter := verify.NewReporter()
	report := reporter.Generate(results, verifyResults, time.Since(startedAt))

	finalStatus := "completed"
	if report.Failed > 0 {
		finalStatus = "failed"
	}
	if err := store.CompleteRun(runID, finalStatus, time.Now()); err != nil {
		return nil, err
	}

	notify(WorkflowEvent{Stage: "complete", Message: "流程结束", Successes: report.Succeeded, Failures: report.Failed, Total: report.TotalCases})
	return &WorkflowResult{
		Mode:     "start",
		Dir:      dir,
		Analyses: analyses,
		Results:  results,
		Verifies: verifyResults,
		Report:   report,
	}, nil
}

type planItemMeta struct {
	File       string
	TargetFile string
	Kind       string
	SourceHash string
}

type migrationPlan struct {
	TotalPlanned     int
	Skipped          []planItemMeta
	AlreadyMigrated  []planItemMeta
	ToExecuteHelpers []*types.FullAnalysis
	ToExecuteCases   []*types.FullAnalysis
	MetaByFile      map[string]planItemMeta
}

func buildMigrationPlan(engine *analyzer.Engine, analyses []*types.FullAnalysis, store *executor.StateStore) (*migrationPlan, error) {
	// Partition: treat files with tests as cases.
	var cases []*types.FullAnalysis
	for _, a := range analyses {
		if isCaseAnalysis(a) {
			cases = append(cases, a)
		}
	}

	// Discover helper candidates from all cases.
	helperPaths := make(map[string]struct{})
	for _, ca := range cases {
		deps := collectLocalImportDeps(ca)
		funcNames := collectCandidateFuncNames(ca)
		scanned := scanHelperCandidates(filepath.Dir(ca.FilePath), funcNames)
		chainDeps := tracePropertyChainDeps(ca)

		// Dependencies provide context; helper candidates may be migrated.
		ca.Dependencies = uniqueStrings(append(append(deps, scanned...), chainDeps...))
		for _, p := range scanned {
			helperPaths[p] = struct{}{}
		}
		for _, p := range chainDeps {
			helperPaths[p] = struct{}{}
		}
		for _, p := range deps {
			// If a local import is NEP-related, it should also be migrated.
			if isNepRelatedFile(p) {
				helperPaths[p] = struct{}{}
			}
		}
	}

	// Analyze helper files, skipping those already migrated to Midscene.
	var helpers []*types.FullAnalysis
	var alreadyMigratedMetas []planItemMeta
	defaultPromptsByFile := make(map[string][]types.DefaultPromptInfo)
	for p := range helperPaths {
		a, err := engine.AnalyzeFile(p)
		if err != nil {
			// Keep best-effort: skip analysis failure of helper candidates.
			continue
		}
		// Add its own local deps for context.
		a.Dependencies = uniqueStrings(collectLocalImportDeps(a))
		// Extract component DEFAULT_PROMPTs for later use in case prompt generation.
		a.DefaultPrompts = analyzer.ExtractDefaultPrompts(a.FilePath)
		if len(a.DefaultPrompts) > 0 {
			defaultPromptsByFile[a.FilePath] = a.DefaultPrompts
		}

		// Check if this helper has already been migrated at its target path.
		helperFuncNames := extractDefinedFuncNames(a)
		if isHelperAlreadyMigrated(a.TargetPath, helperFuncNames) {
			hash, _ := sha256File(a.FilePath)
			alreadyMigratedMetas = append(alreadyMigratedMetas, planItemMeta{
				File:       a.FilePath,
				TargetFile: a.TargetPath,
				Kind:       "helper",
				SourceHash: hash,
			})
			continue
		}

		helpers = append(helpers, a)
	}

	// Attach DEFAULT_PROMPT entries from dependencies to each case so the prompt
	// can guide AI migration for NEP component wrappers.
	for _, ca := range cases {
		if ca == nil {
			continue
		}
		seenKey := make(map[string]struct{})
		for _, dp := range ca.DefaultPrompts {
			k := dp.ClassName + "|" + dp.PromptValue + "|" + dp.FilePath
			seenKey[k] = struct{}{}
		}
		for _, dep := range ca.Dependencies {
			prompts := defaultPromptsByFile[filepath.Clean(dep)]
			for _, dp := range prompts {
				k := dp.ClassName + "|" + dp.PromptValue + "|" + dp.FilePath
				if _, ok := seenKey[k]; ok {
					continue
				}
				seenKey[k] = struct{}{}
				ca.DefaultPrompts = append(ca.DefaultPrompts, dp)
			}
		}
	}

	// Dedup helpers/cases by absolute path.
	helperByFile := make(map[string]*types.FullAnalysis)
	for _, h := range helpers {
		if h == nil {
			continue
		}
		fp := filepath.Clean(h.FilePath)
		helperByFile[fp] = h
	}
	caseByFile := make(map[string]*types.FullAnalysis)
	for _, c := range cases {
		if c == nil {
			continue
		}
		fp := filepath.Clean(c.FilePath)
		caseByFile[fp] = c
	}

	plan := &migrationPlan{MetaByFile: make(map[string]planItemMeta), AlreadyMigrated: alreadyMigratedMetas}
	// preserve a stable order for execution.
	sortedHelpers := make([]string, 0, len(helperByFile))
	for fp := range helperByFile {
		sortedHelpers = append(sortedHelpers, fp)
	}
	sort.Strings(sortedHelpers)
	sortedCases := make([]string, 0, len(caseByFile))
	for fp := range caseByFile {
		sortedCases = append(sortedCases, fp)
	}
	sort.Strings(sortedCases)

	// Resolve up-to-date tasks (cross-run dedupe) and build metadata for progress/state.
	addPlanned := func(kind string, a *types.FullAnalysis, toExecute *[]*types.FullAnalysis) {
		if a == nil {
			return
		}
		hash, err := sha256File(a.FilePath)
		if err != nil {
			hash = ""
		}
		meta := planItemMeta{File: a.FilePath, Kind: kind, TargetFile: a.TargetPath, SourceHash: hash}
		plan.MetaByFile[a.FilePath] = meta
		plan.TotalPlanned++
		if store != nil && hash != "" && store.IsUpToDate(a.FilePath, hash, a.TargetPath) {
			plan.Skipped = append(plan.Skipped, meta)
			return
		}
		*toExecute = append(*toExecute, a)
	}

	for _, fp := range sortedHelpers {
		addPlanned("helper", helperByFile[fp], &plan.ToExecuteHelpers)
	}
	for _, fp := range sortedCases {
		addPlanned("case", caseByFile[fp], &plan.ToExecuteCases)
	}

	// Ensure skipped list order is stable.
	sort.Slice(plan.Skipped, func(i, j int) bool { return plan.Skipped[i].File < plan.Skipped[j].File })
	return plan, nil
}

func isCaseAnalysis(a *types.FullAnalysis) bool {
	if a == nil || a.AST == nil {
		return false
	}
	for _, fn := range a.AST.Functions {
		if fn.IsTest {
			return true
		}
	}
	return false
}

func collectLocalImportDeps(a *types.FullAnalysis) []string {
	if a == nil || a.AST == nil {
		return nil
	}
	var deps []string
	for _, imp := range a.AST.Imports {
		if !strings.HasPrefix(imp.Path, ".") {
			continue
		}
		resolved := resolveLocalImportFile(a.FilePath, imp.Path)
		if resolved == "" {
			continue
		}
		deps = append(deps, resolved)
	}
	return uniqueStrings(deps)
}

func collectCandidateFuncNames(a *types.FullAnalysis) []string {
	ignore := map[string]struct{}{
		"it": {}, "test": {}, "describe": {}, "beforeeach": {}, "aftereach": {}, "expect": {},
		"action": {}, "getelement": {}, // common nep/core names; too noisy for scanning
	}
	seen := make(map[string]struct{})
	var names []string
	for _, chain := range a.CallChains {
		for _, step := range chain.Steps {
			if step.IsNepAPI || step.IsNep {
				continue
			}
			name := strings.TrimSpace(step.FuncName)
			if name == "" {
				name = lastIdent(step.Callee)
			}
			lower := strings.ToLower(name)
			if lower == "" {
				continue
			}
			if _, ok := ignore[lower]; ok {
				continue
			}
			if len(lower) < 4 {
				continue
			}
			if _, ok := seen[lower]; ok {
				continue
			}
			seen[lower] = struct{}{}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func scanHelperCandidates(caseDir string, funcNames []string) []string {
	if caseDir == "" || len(funcNames) == 0 {
		return nil
	}
	// limit scan to case dir and common subdirs
	subdirs := []string{".", "commonActions", "actions", "helpers", "helper", "po", "pageObjects", "pageobjects", "pages"}
	var dirs []string
	for _, sd := range subdirs {
		p := filepath.Clean(filepath.Join(caseDir, sd))
		info, err := os.Stat(p)
		if err == nil && info.IsDir() {
			dirs = append(dirs, p)
		}
	}
	dirs = uniqueStrings(dirs)

	var hits []string
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !isScriptFile(name) {
				continue
			}
			path := filepath.Join(d, name)
			if strings.Contains(path, string(filepath.Separator)+"node_modules"+string(filepath.Separator)) {
				continue
			}
			if strings.Contains(path, string(filepath.Separator)+"dist"+string(filepath.Separator)) {
				continue
			}
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			text := string(content)
			if !isNepRelatedText(text) {
				continue
			}
			for _, fn := range funcNames {
				if fn == "" {
					continue
				}
				if strings.Contains(text, fn+"(") || strings.Contains(text, "function "+fn) || strings.Contains(text, fn+":") {
					hits = append(hits, path)
					break
				}
			}
		}
	}
	return uniqueStrings(hits)
}

// tracePropertyChainDeps discovers helper/module/component files referenced via
// multi-level property access chains in a case, e.g.:
//   adGroupPage.optimizationAndBiddingModule1MNBA.vv_setStandardBtn()
// The core goal is to find the PageObject/module files that define the
// intermediate property (optimizationAndBiddingModule1MNBA) so they can be
// migrated before the case.
func tracePropertyChainDeps(ca *types.FullAnalysis) []string {
	props := collectWrapperPropertyNames(ca)
	if len(props) == 0 {
		return nil
	}
	roots := findPageObjectSearchRoots(ca.FilePath)
	if len(roots) == 0 {
		roots = []string{filepath.Dir(ca.FilePath)}
	}

	seen := make(map[string]struct{})
	add := func(p string, out *[]string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		p = filepath.Clean(p)
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		*out = append(*out, p)
	}

	var discovered []string
	for _, prop := range props {
		candidates := scanFilesForProperty(roots, prop)
		for _, poFile := range candidates {
			add(poFile, &discovered)
			moduleFile := resolveModuleFileFromProperty(poFile, prop)
			if moduleFile != "" {
				add(moduleFile, &discovered)
			}
		}
	}

	return uniqueStrings(discovered)
}

func collectWrapperPropertyNames(ca *types.FullAnalysis) []string {
	if ca == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var props []string
	for _, chain := range ca.CallChains {
		for _, step := range chain.Steps {
			if !step.IsWrapperCall {
				continue
			}
			full := strings.TrimSpace(step.FullReceiver)
			if full == "" {
				continue
			}
			parts := strings.Split(full, ".")
			if len(parts) < 2 {
				continue
			}
			prop := strings.TrimSpace(parts[len(parts)-1])
			if prop == "" {
				continue
			}
			if _, ok := seen[prop]; ok {
				continue
			}
			seen[prop] = struct{}{}
			props = append(props, prop)
		}
	}
	sort.Strings(props)
	return props
}

func findPageObjectSearchRoots(caseFilePath string) []string {
	if caseFilePath == "" {
		return nil
	}
	caseDir := filepath.Dir(caseFilePath)

	var roots []string
	seen := make(map[string]struct{})
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			return
		}
		seen[p] = struct{}{}
		roots = append(roots, p)
	}

	// Walk up a few levels to find common roots like e2e/pages, pages, pageObjects.
	cur := caseDir
	for i := 0; i < 8; i++ {
		add(filepath.Join(cur, "pages"))
		add(filepath.Join(cur, "pageObjects"))
		add(filepath.Join(cur, "pageobjects"))
		add(filepath.Join(cur, "new_pages"))
		base := filepath.Base(cur)
		if base == "e2e" {
			add(filepath.Join(cur, "pages"))
			add(filepath.Join(cur, "pageObjects"))
			add(filepath.Join(cur, "pageobjects"))
			add(filepath.Join(cur, "pages", "new_pages"))
			add(filepath.Join(cur, "pages", "new_pages"))
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	return uniqueStrings(roots)
}

func scanFilesForProperty(roots []string, propertyName string) []string {
	propertyName = strings.TrimSpace(propertyName)
	if propertyName == "" {
		return nil
	}
	// Very lightweight filter to reduce false positives.
	lineRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(propertyName) + `\b`)

	maxDepth := 10
	maxFiles := 4000
	filesScanned := 0

	seen := make(map[string]struct{})
	var hits []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" || name == "dist" {
					return filepath.SkipDir
				}
				rel, relErr := filepath.Rel(root, path)
				if relErr == nil && rel != "." {
					if strings.Count(rel, string(filepath.Separator)) >= maxDepth {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !isScriptFile(d.Name()) {
				return nil
			}
			filesScanned++
			if filesScanned > maxFiles {
				return filepath.SkipDir
			}
			b, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			text := string(b)
			if !strings.Contains(text, propertyName) {
				return nil
			}
			// Require the token to appear in a plausible property declaration/assignment.
			for _, line := range strings.Split(text, "\n") {
				if !lineRe.MatchString(line) {
					continue
				}
				if strings.Contains(line, propertyName+":") || strings.Contains(line, propertyName+" =") || strings.Contains(line, propertyName+"=") {
					clean := filepath.Clean(path)
					if _, ok := seen[clean]; !ok {
						seen[clean] = struct{}{}
						hits = append(hits, clean)
					}
					break
				}
			}
			return nil
		})
	}

	sort.Strings(hits)
	return hits
}

func resolveModuleFileFromProperty(pageObjectFile string, propertyName string) string {
	typeName, importPath := findPropertyTypeAndImport(pageObjectFile, propertyName)
	if typeName == "" {
		return ""
	}
	if importPath == "" {
		return ""
	}
	if !strings.HasPrefix(importPath, ".") {
		return ""
	}
	return resolveLocalImportFile(pageObjectFile, importPath)
}

func findPropertyTypeAndImport(filePath, propertyName string) (string, string) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return "", ""
	}
	text := string(b)
	lines := strings.Split(text, "\n")

	propertyName = strings.TrimSpace(propertyName)
	if propertyName == "" {
		return "", ""
	}

	// Try to infer the type from either a type annotation or a `new TypeName(...)` assignment.
	typeRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(propertyName) + `\s*\??\s*:\s*([A-Za-z_$][\w$]*)`)
	newRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(propertyName) + `\s*=\s*new\s+([A-Za-z_$][\w$]*)`)

	typeName := ""
	for _, line := range lines {
		if !strings.Contains(line, propertyName) {
			continue
		}
		if m := typeRe.FindStringSubmatch(line); len(m) == 2 {
			typeName = m[1]
			break
		}
		if m := newRe.FindStringSubmatch(line); len(m) == 2 {
			typeName = m[1]
			break
		}
	}
	if typeName == "" {
		return "", ""
	}

	// Find the import path for this type.
	importRe := regexp.MustCompile(`^\s*import\s+([^;]+?)\s+from\s+['"]([^'"]+)['"]`)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "import ") {
			continue
		}
		m := importRe.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		spec := strings.TrimSpace(m[1])
		path := strings.TrimSpace(m[2])
		if importSpecIncludesSymbol(spec, typeName) {
			return typeName, path
		}
	}

	return typeName, ""
}

func importSpecIncludesSymbol(importSpec string, symbol string) bool {
	importSpec = strings.TrimSpace(importSpec)
	symbol = strings.TrimSpace(symbol)
	if importSpec == "" || symbol == "" {
		return false
	}

	// Default import: import Foo from '...'
	if !strings.HasPrefix(importSpec, "{") {
		fields := strings.Fields(importSpec)
		if len(fields) > 0 && fields[0] == symbol {
			return true
		}
		// handle `Foo, { Bar }` forms
		if strings.Contains(importSpec, ",") {
			parts := strings.Split(importSpec, ",")
			if len(parts) > 0 {
				def := strings.TrimSpace(parts[0])
				if def == symbol {
					return true
				}
			}
		}
	}

	// Named imports: import { A, B as C } from '...'
	if strings.Contains(importSpec, "{") {
		start := strings.Index(importSpec, "{")
		end := strings.LastIndex(importSpec, "}")
		if start >= 0 && end > start {
			inside := importSpec[start+1 : end]
			items := strings.Split(inside, ",")
			for _, it := range items {
				it = strings.TrimSpace(it)
				if it == "" {
					continue
				}
				// handle `A as B`
				parts := strings.Fields(it)
				if len(parts) == 1 && parts[0] == symbol {
					return true
				}
				if len(parts) >= 3 && parts[0] == symbol {
					return true
				}
				if len(parts) >= 3 && parts[len(parts)-1] == symbol {
					return true
				}
			}
		}
	}

	return false
}

func isNepRelatedFile(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return isNepRelatedText(string(b))
}

func isNepRelatedText(text string) bool {
	// Keep this conservative to avoid pulling in selector-only utilities.
	markers := []string{"ai.action(", "ai?.action(", "ai.getElement(", "ai?.getElement(", "clickElementByVL("}
	for _, m := range markers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

// isMidsceneText checks whether file content contains Midscene agent API markers,
// indicating the file has already been migrated to the Midscene framework.
func isMidsceneText(text string) bool {
	markers := []string{
		"agent.aiTap(",
		"agent.aiInput(",
		"agent.aiAssert(",
		"agent.aiWaitFor(",
		"agent.aiHover(",
		"agent.aiScroll(",
		"agent.aiAction(",
		"agent.aiAct(",
		"midscene.NewPage(",
	}
	for _, m := range markers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

// isHelperAlreadyMigrated checks whether a helper file has already been migrated
// to the Midscene framework at its target path. It verifies that:
//  1. The target file exists and is readable.
//  2. The target file contains Midscene agent API calls.
//  3. The target file no longer contains NEP API calls (fully migrated).
//  4. The target file contains at least one of the expected function definitions.
func isHelperAlreadyMigrated(targetPath string, funcNames []string) bool {
	if targetPath == "" {
		return false
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		return false // target file doesn't exist yet
	}
	text := string(content)

	// Must contain Midscene markers.
	if !isMidsceneText(text) {
		return false
	}

	// Must NOT still contain NEP markers (otherwise only partially migrated).
	if isNepRelatedText(text) {
		return false
	}

	// If no specific function names to check, framework-level check is sufficient.
	if len(funcNames) == 0 {
		return true
	}

	// Must contain at least one of the expected function definitions.
	for _, fn := range funcNames {
		if fn == "" {
			continue
		}
		if strings.Contains(text, fn+"(") ||
			strings.Contains(text, "function "+fn) ||
			strings.Contains(text, fn+":") {
			return true
		}
	}
	return false
}

// extractDefinedFuncNames extracts non-test function names from a helper file's AST analysis.
func extractDefinedFuncNames(a *types.FullAnalysis) []string {
	if a == nil || a.AST == nil {
		return nil
	}
	var names []string
	for _, fn := range a.AST.Functions {
		if fn.Name != "" && !fn.IsTest {
			names = append(names, fn.Name)
		}
	}
	return names
}

func isScriptFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts":
		return true
	default:
		return false
	}
}

func lastIdent(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	// strip call args
	if idx := strings.Index(expr, "("); idx >= 0 {
		expr = expr[:idx]
	}
	// strip property access
	parts := strings.Split(expr, ".")
	return strings.TrimSpace(parts[len(parts)-1])
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		it = filepath.Clean(it)
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	sort.Strings(out)
	return out
}

func resolveLocalImportFile(baseFile, importPath string) string {
	baseDir := filepath.Dir(baseFile)
	resolvedBase := filepath.Clean(filepath.Join(baseDir, importPath))
	candidates := buildImportCandidates(resolvedBase)
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func buildImportCandidates(base string) []string {
	if ext := filepath.Ext(base); ext != "" {
		return []string{base}
	}

	exts := []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts"}
	var candidates []string
	for _, ext := range exts {
		candidates = append(candidates, base+ext)
	}
	for _, ext := range exts {
		candidates = append(candidates, filepath.Join(base, "index"+ext))
	}
	return candidates
}

func sha256File(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func buildExecutionLogLines(result *types.MigrationResult, tool string) []string {
	if result == nil {
		return nil
	}

	status := "失败"
	if result.Success {
		status = "成功"
	}

	lines := []string{
		fmt.Sprintf("[%s] %s", status, result.CaseFile),
	}

	output := strings.TrimSpace(result.Output)
	if output != "" {
		for _, line := range strings.Split(prettyJSON(output), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			prefix := strings.TrimSpace(tool)
			if prefix == "" {
				prefix = "coco"
			}
			lines = append(lines, prefix+"> "+line)
		}
	}

	if result.Error != "" {
		lines = append(lines, "error> "+result.Error)
	}

	return lines
}

func prettyJSON(raw string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return raw
	}
	return buf.String()
}

func (r *AppRuntime) LoadState(dir string) (executor.StateSnapshot, error) {
	// Load state from project root, not from the selected directory.
	store, err := executor.NewStateStore(r.cfg.ConfigDir())
	if err != nil {
		return executor.StateSnapshot{}, err
	}
	return store.Snapshot(), nil
}

// NoopRuntime is used in unit tests.
type NoopRuntime struct{}

func NewNoopRuntime() *NoopRuntime {
	return &NoopRuntime{}
}

func (r *NoopRuntime) ListDirectories(root string) ([]string, error) {
	return []string{root}, nil
}

func (r *NoopRuntime) ListImmediateDirectories(path string) ([]string, error) {
	return nil, nil
}

func (r *NoopRuntime) RunAnalyze(ctx context.Context, dir string, notify func(WorkflowEvent)) (*WorkflowResult, error) {
	notify(WorkflowEvent{Stage: "analyze", Message: "noop analyze", Total: 0})
	return &WorkflowResult{Mode: "analyze", Dir: dir}, nil
}

func (r *NoopRuntime) RunStart(ctx context.Context, dir string, targetBaseDir string, notify func(WorkflowEvent)) (*WorkflowResult, error) {
	notify(WorkflowEvent{Stage: "complete", Message: "noop start", Total: 0})
	return &WorkflowResult{Mode: "start", Dir: dir}, nil
}

func (r *NoopRuntime) LoadState(dir string) (executor.StateSnapshot, error) {
	return executor.StateSnapshot{}, nil
}
