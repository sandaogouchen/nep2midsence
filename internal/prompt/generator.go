package prompt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// Generator produces structured migration prompts for Coco
type Generator struct {
	cfg             *config.Config
	migrationDoc    string
	tmpl            *template.Template
	maxPromptTokens int
}

// PromptData holds all data needed to render a migration prompt
type PromptData struct {
	MigrationDoc   string
	SourceFile     string
	TargetFile     string
	RelatedFiles   []string
	ReferenceDocs  []string
	APIMappings    []MappingEntry
	OperationSteps []OperationStep
	DefaultPrompts []DefaultPromptEntry
	DataFlows      []DataFlowEntry
	Patterns       []PatternEntry
	StepIntents    []IntentEntry
	CodeFenceLang  string
	TargetDir      string
	Constraints    []string
	ExampleBefore  string
	ExampleAfter   string
}

type MappingEntry struct {
	NepAPI      string
	MidsceneAPI string
	Note        string
}

type OperationStep struct {
	Index        int
	StepType     string
	Intent       string
	NepCall      string
	MidsceneCall string
}

type DefaultPromptEntry struct {
	ClassName   string
	PromptValue string
	FilePath    string
}

type DataFlowEntry struct {
	Variable  string
	Kind      string
	Value     string
	DefinedAt string
}

type PatternEntry struct {
	Type     string
	Strategy string
}

type IntentEntry struct {
	StepIndex  int
	NepCall    string
	Intent     string
	Source     string
	Confidence float64
}

func NewGenerator(cfg *config.Config) *Generator {
	g := &Generator{
		cfg:             cfg,
		migrationDoc:    strings.TrimSpace(defaultMigrationDoc),
		maxPromptTokens: 100000, // ~100k tokens limit
	}

	// Load custom migration doc if configured.
	if cfg.MigrationDoc != "" {
		data, err := os.ReadFile(cfg.MigrationDoc)
		if err == nil {
			g.migrationDoc = string(data)
		}
	}

	// Parse template
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}
	g.tmpl = template.Must(template.New("migration").Funcs(funcMap).Parse(migrationTemplate))

	return g
}

// Generate creates a structured migration prompt from analysis results
func (g *Generator) Generate(analysis *types.FullAnalysis) (string, error) {
	data := g.buildPromptData(analysis)

	// Estimate token count (rough: 1 token ~ 4 chars)
	estimated := g.estimateTokens(data)

	if estimated > g.maxPromptTokens {
		// Strategy 1: truncate related file list, let Coco Grep/Read on demand
		if len(data.RelatedFiles) > 60 {
			data.RelatedFiles = data.RelatedFiles[:60]
			data.Constraints = append(data.Constraints,
				"相关文件过多，已截断列表；请用 Grep 定位并按需 Read 其它依赖")
		}

		// Strategy 2: truncate examples
		if g.estimateTokens(data) > g.maxPromptTokens {
			if len(data.ExampleBefore) > 200 {
				data.ExampleBefore = data.ExampleBefore[:200] + "\n// ... (truncated)"
			}
			if len(data.ExampleAfter) > 200 {
				data.ExampleAfter = data.ExampleAfter[:200] + "\n// ... (truncated)"
			}
		}
	}

	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render prompt template: %w", err)
	}

	return buf.String(), nil
}

func (g *Generator) buildPromptData(analysis *types.FullAnalysis) *PromptData {
	data := &PromptData{
		MigrationDoc:  g.migrationDoc,
		SourceFile:    analysis.FilePath,
		TargetFile:    analysis.TargetPath,
		CodeFenceLang: detectCodeFenceLanguage(analysis.FilePath, analysis.Language),
	}

	// Provide midscene agent reference doc path (coco should Read it).
	// data.ReferenceDocs = append(data.ReferenceDocs, filepath.Join("internal", "prompt", "assets", "midsence_agent.md"))

	// Build API mappings from annotated calls
	seen := make(map[string]bool)
	for _, ac := range analysis.APIMappings {
		if ac.Rule == nil {
			continue
		}
		key := ac.Rule.NepAPI
		if seen[key] {
			continue
		}
		seen[key] = true
		data.APIMappings = append(data.APIMappings, MappingEntry{
			NepAPI:      ac.Rule.NepAPI,
			MidsceneAPI: ac.Rule.MidsceneEquivalent,
			Note:        ac.Rule.Note,
		})
	}

	// Build operation steps from call chains
	stepIdx := 0
	for _, chain := range analysis.CallChains {
		// Build intent lookup for this chain: StepIndex refers to index within chain.Steps.
		intentByStepIndex := make(map[int]string)
		for _, intentInfo := range analysis.Intents {
			if intentInfo == nil || intentInfo.FuncName != chain.EntryFunc {
				continue
			}
			for _, si := range intentInfo.StepIntents {
				intentByStepIndex[si.StepIndex] = si.InferredIntent
			}
		}

		for localIdx, step := range chain.Steps {
			if !step.IsNepAPI && !step.IsWrapperCall {
				continue
			}

			stepType := "NEP API"
			if step.IsWrapperCall && !step.IsNepAPI {
				stepType = "封装方法"
			}

			midsceneCall := ""
			if step.MigrationRule != nil {
				midsceneCall = step.MigrationRule.MidsceneEquivalent
			} else if stepType == "封装方法" {
				midsceneCall = "（需新建 Midscene 版本的重写函数，不要修改原函数；case 中改为调用新函数）"
			}

			intent := strings.TrimSpace(intentByStepIndex[localIdx])
			if intent == "" {
				// Prefer inline comment as an intent hint.
				if strings.TrimSpace(step.Comment) != "" {
					intent = strings.TrimSpace(step.Comment)
				} else {
					intent = step.Callee + "(" + strings.Join(step.Args, ", ") + ")"
				}
			}

			data.OperationSteps = append(data.OperationSteps, OperationStep{
				Index:        stepIdx,
				StepType:     stepType,
				Intent:       intent,
				NepCall:      step.Callee + "(" + strings.Join(step.Args, ", ") + ")",
				MidsceneCall: midsceneCall,
			})
			stepIdx++
		}
	}

	// Build DEFAULT_PROMPT entries (filled when analysis.DefaultPrompts is present)
	for _, dp := range analysis.DefaultPrompts {
		data.DefaultPrompts = append(data.DefaultPrompts, DefaultPromptEntry{
			ClassName:   dp.ClassName,
			PromptValue: dp.PromptValue,
			FilePath:    dp.FilePath,
		})
	}

	// Build data flow entries
	for _, df := range analysis.DataFlows {
		data.DataFlows = append(data.DataFlows, DataFlowEntry{
			Variable:  df.Variable,
			Kind:      string(df.Kind),
			Value:     df.Value,
			DefinedAt: fmt.Sprintf("%s:%d", df.DefinedAt.File, df.DefinedAt.Line),
		})
	}

	// Build pattern entries
	if analysis.Patterns != nil {
		for _, pt := range analysis.Patterns.Detected {
			strategy := analysis.Patterns.Strategies[pt]
			data.Patterns = append(data.Patterns, PatternEntry{
				Type:     string(pt),
				Strategy: strategy,
			})
		}
	}

	// Build intent entries
	for _, intentInfo := range analysis.Intents {
		for _, si := range intentInfo.StepIntents {
			data.StepIntents = append(data.StepIntents, IntentEntry{
				StepIndex:  si.StepIndex,
				NepCall:    si.NepAPICall,
				Intent:     si.InferredIntent,
				Source:     si.IntentSource,
				Confidence: si.Confidence,
			})
		}
	}

	// Path-only mode: do not inline any source code into the prompt.
	// Coco can access files by path, so we only provide file paths here.
	data.RelatedFiles = append(data.RelatedFiles, analysis.FilePath)
	if analysis.Dependencies != nil {
		data.RelatedFiles = append(data.RelatedFiles, analysis.Dependencies...)
	}
	data.RelatedFiles = uniqueNonEmpty(data.RelatedFiles)

	// Build constraints
	data.Constraints = []string{
		fmt.Sprintf("将迁移后的代码写入: %s", analysis.TargetPath),
		"保持文件名不变",
		"保留所有注释，更新为 midscene 对应写法",
		"迁移前先 Read 参考文档: ",
		"只修改 NEP 相关代码：把 nep 的调用迁移为 midscene.agent 的等价写法；不要重排无关逻辑",
		"Pagepass 原生 selector 方式的代码保持不变（例如使用 selector/locator 的直接操作、原生断言/等待等）",
		`若遇到封装函数（如 commonActions / pageObject 方法），采用"新建重写函数"策略，禁止直接修改或替换原有封装函数：
   a. 在封装文件中新建一个带 Midscene 后缀（或其他可区分命名）的新函数，例如原函数为 editCampaign2() 则新建 editCampaign2Midscene()
   b. 新函数内部使用 midscene agent API 重写原函数的完整逻辑流程，保持业务语义一致
   c. 原有封装函数保持不变，不做任何修改
   d. 在迁移后的 case 文件中，将对原封装函数的调用替换为对新函数的调用（如 listPage.commonActions.editCampaign2() → listPage.commonActions.editCampaign2Midscene()）
   e. 新函数需要接收 agent 参数（从 case 传入），签名示例：async editCampaign2Midscene(agent: AgentWI)
   f. 同一封装文件中，对同一个原函数只新建一次重写函数，避免重复`,
		"判断封装函数是否已有 Midscene 版本：若封装文件中已存在对应的 Midscene 后缀函数（如 editCampaign2Midscene），则直接在 case 中调用该函数，无需再次新建",
		"不要修改原始文件",
		"迁移完成后检查代码是否有语法错误",
	}

	return data
}

func uniqueNonEmpty(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	sort.Strings(out)
	return out
}

func detectCodeFenceLanguage(filePath, language string) string {
	switch strings.ToLower(language) {
	case "typescript":
		return "typescript"
	case "javascript":
		return "javascript"
	case "go":
		return "go"
	}

	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	default:
		return "go"
	}
}

func (g *Generator) buildRelatedCode(analysis *types.FullAnalysis) string {
	if analysis == nil || analysis.AST == nil {
		return ""
	}

	var sections []string
	sections = append(sections, g.buildLocalFunctionContext(analysis)...)
	sections = append(sections, g.buildImportedFileContext(analysis)...)

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func (g *Generator) buildLocalFunctionContext(analysis *types.FullAnalysis) []string {
	if analysis == nil || analysis.AST == nil {
		return nil
	}

	content, err := os.ReadFile(analysis.FilePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(content), "\n")
	relevant := collectRelevantFunctions(analysis)
	if len(relevant) == 0 {
		return nil
	}

	var sections []string
	for _, fn := range analysis.AST.Functions {
		reasons, ok := relevant[fn.Name]
		if !ok {
			continue
		}
		snippet := extractLineRange(lines, fn.LineStart, fn.LineEnd)
		if strings.TrimSpace(snippet) == "" {
			continue
		}
		sections = append(sections, formatRelatedCodeSection(
			fmt.Sprintf("当前 case 的关联函数 `%s`", fn.Name),
			analysis.FilePath,
			fn.LineStart,
			fn.LineEnd,
			joinReasons(reasons),
			detectCodeFenceLanguage(analysis.FilePath, analysis.Language),
			snippet,
		))
	}

	return sections
}

func (g *Generator) buildImportedFileContext(analysis *types.FullAnalysis) []string {
	if analysis == nil || analysis.AST == nil {
		return nil
	}

	var sections []string
	seen := make(map[string]struct{})
	for _, imp := range analysis.AST.Imports {
		if !isLocalImportPath(imp.Path) {
			continue
		}
		resolved := resolveLocalImportFile(analysis.FilePath, imp.Path)
		if resolved == "" {
			continue
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}

		code, endLine, truncated := readRelatedFileSnippet(resolved, 220)
		if strings.TrimSpace(code) == "" {
			continue
		}
		reason := "由 AST import 解析得到的本地依赖代码，可补充 helper / page object 上下文"
		if truncated {
			reason += "；文件较长，已截断为前 220 行"
		}
		sections = append(sections, formatRelatedCodeSection(
			fmt.Sprintf("本地依赖 `%s`", imp.Path),
			resolved,
			1,
			endLine,
			reason,
			detectCodeFenceLanguage(resolved, ""),
			code,
		))
	}

	return sections
}

func collectRelevantFunctions(analysis *types.FullAnalysis) map[string][]string {
	reasons := make(map[string]map[string]struct{})
	add := func(name, reason string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := reasons[name]; !ok {
			reasons[name] = make(map[string]struct{})
		}
		reasons[name][reason] = struct{}{}
	}

	for _, chain := range analysis.CallChains {
		add(chain.EntryFunc, "调用链入口")
		add(chain.TestFunc, "测试入口")
		for _, step := range chain.Steps {
			add(step.Caller, "调用链中的调用方")
			add(step.InFunc, "NEP 调用所在函数")
		}
		for _, step := range chain.NepAPICalls {
			add(step.Caller, "NEP API 的直接调用方")
			add(step.InFunc, "NEP API 所在函数")
		}
	}

	result := make(map[string][]string, len(reasons))
	for name, items := range reasons {
		result[name] = joinReasonMap(items)
	}
	return result
}

func joinReasonMap(items map[string]struct{}) []string {
	result := make([]string, 0, len(items))
	for item := range items {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func joinReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return strings.Join(reasons, "；")
}

func extractLineRange(lines []string, startLine, endLine int) string {
	if startLine <= 0 || endLine <= 0 || startLine > endLine || startLine > len(lines) {
		return ""
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	return strings.TrimSpace(strings.Join(lines[startLine-1:endLine], "\n"))
}

func formatRelatedCodeSection(title, path string, startLine, endLine int, reason, language, code string) string {
	var builder strings.Builder
	builder.WriteString("#### ")
	builder.WriteString(title)
	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("- 文件: `%s:%d`\n", path, startLine))
	if endLine >= startLine {
		builder.WriteString(fmt.Sprintf("- 行号: %d-%d\n", startLine, endLine))
	}
	if reason != "" {
		builder.WriteString("- 关联原因: ")
		builder.WriteString(reason)
		builder.WriteString("\n")
	}
	builder.WriteString("\n```")
	builder.WriteString(language)
	builder.WriteString("\n")
	builder.WriteString(strings.TrimSpace(code))
	builder.WriteString("\n```")
	return builder.String()
}

func isLocalImportPath(importPath string) bool {
	return strings.HasPrefix(importPath, ".")
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
	var candidates []string
	if ext := filepath.Ext(base); ext != "" {
		return []string{base}
	}

	exts := []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts"}
	for _, ext := range exts {
		candidates = append(candidates, base+ext)
	}
	for _, ext := range exts {
		candidates = append(candidates, filepath.Join(base, "index"+ext))
	}
	return candidates
}

func readRelatedFileSnippet(path string, maxLines int) (string, int, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", 0, false
	}
	lines := strings.Split(string(content), "\n")
	if maxLines > 0 && len(lines) > maxLines {
		return strings.TrimSpace(strings.Join(lines[:maxLines], "\n")) + "\n// ... (truncated)", maxLines, true
	}
	return strings.TrimSpace(string(content)), len(lines), false
}

func (g *Generator) estimateTokens(data *PromptData) int {
	var buf bytes.Buffer
	g.tmpl.Execute(&buf, data)
	return buf.Len() / 4 // rough estimation: 1 token ~ 4 chars
}

// GenerateRetryPrompt creates an enriched prompt for retry attempts
func (g *Generator) GenerateRetryPrompt(original string, lastError string, attempt int) string {
	return fmt.Sprintf("%s\n\n---\n\n## ⚠️ 重试说明（第 %d 次重试）\n\n上一次执行失败，错误信息：\n\n```\n%s\n```\n\n请根据错误信息修正代码，确保：\n1. 代码语法正确\n2. 所有 import 都被正确使用\n3. 类型匹配正确\n4. 不要遗漏任何必要的迁移步骤\n", original, attempt, lastError)
}
