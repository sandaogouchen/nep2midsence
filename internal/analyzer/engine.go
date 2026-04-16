package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
}

func NewEngine(cfg *config.Config) *Engine {
	customMappings := make(map[string]*types.MigrationRule)
	for apiName, cm := range cfg.CustomMappings {
		customMappings[apiName] = &types.MigrationRule{
			NepAPI:             apiName,
			MidsceneEquivalent: cm.MidsceneEquivalent,
			NeedsIntentRewrite: cm.NeedsIntentRewrite,
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
	}
}

// AnalyzeFile runs the full 5-layer analysis on a single file
func (e *Engine) AnalyzeFile(filePath string) (*types.FullAnalysis, error) {
	result := &types.FullAnalysis{
		FilePath: filePath,
	}

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
	for _, pattern := range e.cfg.Source.FilePatterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
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
