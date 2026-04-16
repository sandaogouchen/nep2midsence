package prompt

import (
	"bytes"
	"fmt"
	"os"
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
	APIMappings    []MappingEntry
	OperationSteps []OperationStep
	DataFlows      []DataFlowEntry
	Patterns       []PatternEntry
	StepIntents    []IntentEntry
	SourceCode     string
	HelperCode     string
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
	Intent       string
	NepCall      string
	MidsceneCall string
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
		maxPromptTokens: 100000, // ~100k tokens limit
	}

	// Load migration doc
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
		// Strategy 1: drop helper code, let Coco read it
		data.HelperCode = ""
		data.Constraints = append(data.Constraints,
			"依赖的 helper 和 PO 代码请通过 Read 工具自行读取")

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
		MigrationDoc: g.migrationDoc,
		SourceFile:   analysis.FilePath,
		TargetFile:   analysis.TargetPath,
	}

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
		for _, step := range chain.NepAPICalls {
			midsceneCall := ""
			if step.MigrationRule != nil {
				midsceneCall = step.MigrationRule.MidsceneEquivalent
			}
			intent := ""
			// Try to find matching intent
			for _, intentInfo := range analysis.Intents {
				for _, si := range intentInfo.StepIntents {
					if si.StepIndex == stepIdx {
						intent = si.InferredIntent
					}
				}
			}
			if intent == "" {
				intent = step.Callee + "(" + strings.Join(step.Args, ", ") + ")"
			}
			data.OperationSteps = append(data.OperationSteps, OperationStep{
				Index:        stepIdx,
				Intent:       intent,
				NepCall:      step.Callee + "(" + strings.Join(step.Args, ", ") + ")",
				MidsceneCall: midsceneCall,
			})
			stepIdx++
		}
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

	// Read source code
	if src, err := os.ReadFile(analysis.FilePath); err == nil {
		data.SourceCode = string(src)
	}

	// Build constraints
	data.Constraints = []string{
		fmt.Sprintf("将迁移后的代码写入: %s", analysis.TargetPath),
		"保持文件名不变",
		"保留所有注释，更新为 midscene 对应写法",
		"不要修改原始文件",
		"迁移完成后检查代码是否有语法错误",
	}

	return data
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
