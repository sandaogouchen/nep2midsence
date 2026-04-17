package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/fingerprint"
	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// Engine orchestrates the 5-layer analysis pipeline
type Engine struct {
	cfg               *config.Config
	astAnalyzer       *ASTAnalyzer
	callGraphAnalyzer *CallGraphAnalyzer
	dataFlowAnalyzer  *DataFlowAnalyzer
	patternDetector   *PatternDetector
	intentAnalyzer    *IntentAnalyzer
	annotator         *fingerprint.Annotator
	tsBridge          *TSBridge
}

func NewEngine(cfg *config.Config) *Engine {
	customMappings := make(map[string]*types.MigrationRule)
	for apiName, cm := range cfg.CustomMappings {
		// cfg.CustomMappings is map[string]string where the value is
		// the midscene equivalent API name. Wrap it into a MigrationRule.
		customMappings[apiName] = &types.MigrationRule{
			NepAPI:             apiName,
			MidsceneEquivalent: cm,
			NeedsIntentRewrite: false,
		}
	}

	return &Engine{
		cfg:               cfg,
		astAnalyzer:       NewASTAnalyzer(cfg.Source.PackagePrefixes),
		callGraphAnalyzer: NewCallGraphAnalyzer(cfg.Analysis.MaxCallDepth, cfg.Source.PackagePrefixes),
		dataFlowAnalyzer:  NewDataFlowAnalyzer(),
		patternDetector:   NewPatternDetector(),
		intentAnalyzer:    NewIntentAnalyzer(),
		annotator:         fingerprint.NewAnnotator(customMappings),
		tsBridge:          newTSBridgeFromConfig(cfg),
	}
}

// AnalyzeFile runs the full 5-layer analysis on a single file
func (e *Engine) AnalyzeFile(filePath string) (*types.FullAnalysis, error) {
	result := &types.FullAnalysis{
		FilePath: filePath,
	}

	if isTypeScriptLikeFile(filePath) {
		return e.analyzeTypeScriptFile(filePath, result)
	}

	return e.analyzeGoFile(filePath, result)
}

func (e *Engine) analyzeGoFile(filePath string, result *types.FullAnalysis) (*types.FullAnalysis, error) {
	// L1: AST structure
	astInfo, err := e.astAnalyzer.Analyze(filePath)
	if err != nil {
		return nil, fmt.Errorf("L1 AST analysis failed for %s: %w", filePath, err)
	}
	result.AST = astInfo
	result.Package = astInfo.Package

	// Set AST infos for call graph analyzer
	astInfoMap := map[string]*types.ASTInfo{filePath: astInfo}
	e.callGraphAnalyzer.SetASTInfos(astInfoMap)

	// L2: Call graph
	chains := e.callGraphAnalyzer.BuildAllChains(astInfo)
	result.CallChains = chains

	// L6: API fingerprint annotation
	annotated := e.annotator.Annotate(chains)
	result.APIMappings = annotated

	// L3: Data flow (optional)
	if e.cfg.Analysis.EnableDataflow {
		dataFlows := e.dataFlowAnalyzer.Analyze(astInfo, chains)
		result.DataFlows = dataFlows
	}

	// L4: Pattern recognition
	patternResult := e.patternDetector.Detect(astInfo, chains)
	result.Patterns = patternResult
	result.Complexity = patternResult.Complexity

	// L5: Intent inference (optional)
	if e.cfg.Analysis.EnableIntent {
		intents := e.intentAnalyzer.Analyze(chains, astInfo)
		result.Intents = intents
	}

	// Determine target path
	result.TargetPath = e.computeTargetPath(filePath)

	return result, nil
}

func (e *Engine) analyzeTypeScriptFile(filePath string, result *types.FullAnalysis) (*types.FullAnalysis, error) {
	astInfo, allCalls, language, err := e.extractTypeScript(filePath)
	if err != nil {
		return nil, fmt.Errorf("L1 TS analysis failed for %s: %w", filePath, err)
	}

	result.AST = astInfo
	result.Package = astInfo.Package
	result.Language = language

	astInfoMap := map[string]*types.ASTInfo{filePath: astInfo}
	e.callGraphAnalyzer.SetASTInfos(astInfoMap)

	chains := e.callGraphAnalyzer.BuildChainsFromTSCalls(astInfo, allCalls)
	result.CallChains = chains

	annotated := e.annotator.Annotate(chains)
	result.APIMappings = annotated

	if e.cfg.Analysis.EnableDataflow {
		result.DataFlows = e.dataFlowAnalyzer.AnalyzeTS(astInfo, chains)
	}

	patternResult := e.patternDetector.Detect(astInfo, chains)
	e.patternDetector.DetectTS(astInfo, chains, patternResult)
	patternResult.Complexity = classifyPatternComplexity(len(patternResult.Detected))
	result.Patterns = patternResult
	result.Complexity = patternResult.Complexity

	if e.cfg.Analysis.EnableIntent {
		result.Intents = e.intentAnalyzer.Analyze(chains, astInfo)
	}

	result.TargetPath = e.computeTargetPath(filePath)

	return result, nil
}

// AnalyzeDir analyzes all matching files in a directory
func (e *Engine) AnalyzeDir(dir string) ([]*types.FullAnalysis, error) {
	var results []*types.FullAnalysis

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip output directory
			if info.Name() == e.cfg.Target.OutputDir {
				return filepath.SkipDir
			}
			return nil
		}
		if e.matchesPattern(info.Name()) {
			analysis, err := e.AnalyzeFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to analyze %s: %v\n", path, err)
				return nil
			}
			results = append(results, analysis)
		}
		return nil
	})

	return results, err
}

func (e *Engine) matchesPattern(name string) bool {
	// Check FilePatterns and Extensions independently; a match in either is sufficient.
	// This ensures TypeScript files (matched by Extensions) are not excluded when
	// FilePatterns only lists Go-specific globs like "*_test.go".
	hasPatterns := len(e.cfg.Source.FilePatterns) > 0
	hasExtensions := len(e.cfg.Source.Extensions) > 0

	if hasPatterns {
		for _, pattern := range e.cfg.Source.FilePatterns {
			if matched, _ := filepath.Match(pattern, name); matched {
				return true
			}
		}
	}

	if hasExtensions {
		ext := filepath.Ext(name)
		for _, allowed := range e.cfg.Source.Extensions {
			if ext == allowed {
				return true
			}
		}
	}

	// If neither filter is configured, accept all files.
	if !hasPatterns && !hasExtensions {
		return true
	}

	return false
}

func (e *Engine) computeTargetPath(sourcePath string) string {
	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)
	targetDir := filepath.Join(dir, e.cfg.Target.OutputDir)

	if e.cfg.Target.FileSuffix != "" {
		ext := filepath.Ext(base)
		nameWithoutExt := strings.TrimSuffix(base, ext)
		base = nameWithoutExt + e.cfg.Target.FileSuffix + ext
	}

	return filepath.Join(targetDir, base)
}

func (e *Engine) extractTypeScript(filePath string) (*types.ASTInfo, []types.CallStep, string, error) {
	if e.tsBridge != nil && tsBridgeScriptExists(e.tsBridge.scriptPath) {
		results, err := e.tsBridge.Extract([]string{filePath})
		if err == nil && len(results) > 0 {
			astInfo := e.tsBridge.ConvertToASTInfo(results[0])
			return astInfo, e.tsBridge.ConvertAllCalls(results[0]), tsLanguageForPath(filePath), nil
		}
	}

	return extractTypeScriptFallback(filePath)
}

func newTSBridgeFromConfig(cfg *config.Config) *TSBridge {
	if cfg == nil {
		return nil
	}

	timeout := time.Duration(cfg.TSExtractor.Timeout) * time.Second
	return NewTSBridge(cfg.TSExtractor.NodePath, cfg.TSExtractor.ScriptPath, timeout)
}

func isTypeScriptLikeFile(filePath string) bool {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".ts", ".tsx", ".js", ".jsx":
		return true
	default:
		return false
	}
}

func tsLanguageForPath(filePath string) string {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".js", ".jsx":
		return "javascript"
	default:
		return "typescript"
	}
}

func tsBridgeScriptExists(scriptPath string) bool {
	if scriptPath == "" {
		return false
	}
	info, err := os.Stat(scriptPath)
	return err == nil && !info.IsDir()
}

func classifyPatternComplexity(count int) string {
	switch {
	case count <= 1:
		return "simple"
	case count <= 3:
		return "medium"
	default:
		return "complex"
	}
}
