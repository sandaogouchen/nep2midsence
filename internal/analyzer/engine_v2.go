// Package analyzer implements the 5-layer analysis engine for nep2midsence.
//
// engine_v2.go is the full replacement of engine.go. It preserves all existing
// Go analysis functionality and adds language routing so that TypeScript files
// are processed through the TSBridge, with the resulting unified types fed into
// layers L2-L5 unchanged.
package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/fingerprint"
	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// Engine orchestrates the five analysis layers:
//
//	L1 \u2013 AST parsing         (Go: go/ast, TS: TSBridge)
//	L2 \u2013 Call graph building  (CallGraphAnalyzer)
//	L3 \u2013 Data flow tracking   (DataFlowAnalyzer)
//	L4 \u2013 Pattern detection    (PatternDetector)
//	L5 \u2013 Intent inference     (IntentAnalyzer)
type Engine struct {
	cfg               *config.Config
	astAnalyzer       *ASTAnalyzer
	callGraphAnalyzer *CallGraphAnalyzer
	dataFlowAnalyzer  *DataFlowAnalyzer
	patternDetector   *PatternDetector
	intentAnalyzer    *IntentAnalyzer
	annotator         *fingerprint.Annotator
	tsBridge          *TSBridge // nil for Go-only mode
}

// NewEngine creates an Engine configured according to cfg.
// When cfg.Source.Language is "typescript", a TSBridge is initialised so that
// AnalyzeFile/AnalyzeDir can route TS files through the extractor.
func NewEngine(cfg *config.Config) *Engine {
	// Build custom fingerprint mappings by merging top-level and fingerprint-specific.
	customMappings := make(map[string]string)
	for k, v := range cfg.CustomMappings {
		customMappings[k] = v
	}
	for k, v := range cfg.Source.CustomMappings {
		customMappings[k] = v
	}
	for k, v := range cfg.Fingerprint.CustomMappings {
		customMappings[k] = v
	}

	e := &Engine{
		cfg:               cfg,
		astAnalyzer:       NewASTAnalyzer(cfg.Source.PackagePrefixes),
		callGraphAnalyzer: NewCallGraphAnalyzer(cfg.Analysis.MaxCallDepth, cfg.Source.PackagePrefixes),
		dataFlowAnalyzer:  NewDataFlowAnalyzer(),
		patternDetector:   NewPatternDetector(),
		intentAnalyzer:    NewIntentAnalyzer(),
		annotator:         fingerprint.NewAnnotator(customMappings),
	}

	// Initialise TS bridge when the source language is TypeScript.
	if cfg.Source.Language == "typescript" {
		timeout := time.Duration(cfg.TSExtractor.Timeout) * time.Second
		e.tsBridge = NewTSBridge(
			cfg.TSExtractor.NodePath,
			cfg.TSExtractor.ScriptPath,
			timeout,
		)
	}

	return e
}

// ---------------------------------------------------------------------------
// Public entry points
// ---------------------------------------------------------------------------

// AnalyzeFile performs a full 5-layer analysis on a single source file.
// It routes to the Go or TypeScript path based on the file extension and
// the configured source language.
func (e *Engine) AnalyzeFile(filePath string) (*types.FullAnalysis, error) {
	// Language routing
	if e.isTypeScriptFile(filePath) {
		return e.analyzeTypeScriptFile(filePath)
	}

	return e.analyzeGoFile(filePath)
}

// AnalyzeDir walks a directory tree, analyses every matching source file,
// and returns a slice of per-file results. It respects cfg.Source.Exclude,
// cfg.Source.Pattern, and cfg.Source.Extensions.
func (e *Engine) AnalyzeDir(dirPath string) ([]*types.FullAnalysis, error) {
	var results []*types.FullAnalysis

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Skip excluded directories.
		if info.IsDir() {
			base := filepath.Base(path)
			for _, excl := range e.cfg.Source.Exclude {
				if base == excl {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Check file extension.
		if !e.matchesExtension(path) {
			return nil
		}

		// Check glob pattern if configured.
		if e.cfg.Source.Pattern != "" {
			matched, matchErr := filepath.Match(e.cfg.Source.Pattern, filepath.Base(path))
			if matchErr != nil {
				return fmt.Errorf("invalid source pattern %q: %w", e.cfg.Source.Pattern, matchErr)
			}
			if !matched {
				return nil
			}
		}

		analysis, analyzeErr := e.AnalyzeFile(path)
		if analyzeErr != nil {
			// Collect error but keep walking.
			results = append(results, &types.FullAnalysis{
				FilePath: path,
				Language: e.detectLanguage(path),
			})
			return nil
		}
		results = append(results, analysis)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("directory walk failed: %w", err)
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// Go analysis path (existing behaviour, unchanged)
// ---------------------------------------------------------------------------

// analyzeGoFile runs the full L1-L5 pipeline using the Go AST.
func (e *Engine) analyzeGoFile(filePath string) (*types.FullAnalysis, error) {
	result := &types.FullAnalysis{
		FilePath: filePath,
		Language: "go",
	}

	// L1 \u2013 AST parsing
	astInfo, err := e.astAnalyzer.Analyze(filePath)
	if err != nil {
		return nil, fmt.Errorf("L1 AST analysis failed for %s: %w", filePath, err)
	}
	result.AST = astInfo

	// L2 \u2013 Call graph building
	chains, err := e.callGraphAnalyzer.BuildCallChains(astInfo, filePath)
	if err != nil {
		return nil, fmt.Errorf("L2 call graph failed for %s: %w", filePath, err)
	}
	result.CallChains = chains

	// L3 \u2013 Data flow tracking
	dataFlow := e.dataFlowAnalyzer.Analyze(astInfo, chains)
	result.DataFlow = dataFlow

	// L4 \u2013 Pattern detection
	patterns := e.patternDetector.Detect(astInfo, chains)
	result.Patterns = patterns

	// L5 \u2013 Intent inference
	intents := e.intentAnalyzer.Infer(astInfo, chains, patterns, dataFlow)
	result.Intents = intents

	// Fingerprint annotation
	result.Annotation = e.buildAnnotation(patterns, intents)

	return result, nil
}

// ---------------------------------------------------------------------------
// TypeScript analysis path (NEW)
// ---------------------------------------------------------------------------

// analyzeTypeScriptFile runs the TS-adapted pipeline:
// TSBridge \u2192 unified ASTInfo \u2192 L2v2 \u2192 L3v2 \u2192 L4v2 \u2192 L5 \u2192 annotation.
func (e *Engine) analyzeTypeScriptFile(filePath string) (*types.FullAnalysis, error) {
	if e.tsBridge == nil {
		return nil, fmt.Errorf("TypeScript bridge not initialised; set source.language to \"typescript\" in config")
	}

	result := &types.FullAnalysis{
		FilePath: filePath,
		Language: "typescript",
	}

	// Step 1: Extract via TS bridge
	tsResults, err := e.tsBridge.Extract([]string{filePath})
	if err != nil {
		return nil, fmt.Errorf("TS extraction failed for %s: %w", filePath, err)
	}
	if len(tsResults) == 0 {
		return nil, fmt.Errorf("TS extraction returned no results for %s", filePath)
	}

	tsResult := tsResults[0]

	// Step 2: Convert to unified ASTInfo (replaces L1 for TS)
	astInfo := e.tsBridge.ConvertToASTInfo(tsResult)
	result.AST = astInfo

	// Step 3: Get all calls as CallSteps for L2
	tsCallSteps := e.tsBridge.ConvertAllCalls(tsResult)

	// Step 4: Build call chains from pre-extracted calls (skip Go AST walking)
	chains := e.buildChainsFromTSCalls(astInfo, tsCallSteps)
	result.CallChains = chains

	// Step 5: Data flow (L3) \u2013 TS-adapted lightweight analysis
	dataFlow := e.dataFlowAnalyzer.AnalyzeTS(astInfo, chains)
	result.DataFlow = dataFlow

	// Step 6: Pattern detection (L4) \u2013 runs both standard and TS-specific detectors
	patterns := e.patternDetector.Detect(astInfo, chains)
	e.patternDetector.DetectTS(astInfo, chains, patterns)
	result.Patterns = patterns

	// Step 7: Intent inference (L5) \u2013 same as Go path
	intents := e.intentAnalyzer.Infer(astInfo, chains, patterns, dataFlow)
	result.Intents = intents

	// Step 8: Fingerprint annotation
	result.Annotation = e.buildAnnotation(patterns, intents)

	return result, nil
}

// buildChainsFromTSCalls creates CallChains from pre-extracted TypeScript call
// data. Calls are grouped by their containing test function based on line
// ranges from astInfo.Functions where IsTest == true.
func (e *Engine) buildChainsFromTSCalls(astInfo *types.ASTInfo, allCalls []types.CallStep) []*types.CallChain {
	// Delegate to the CallGraphAnalyzer's TS-specific method.
	return e.callGraphAnalyzer.BuildChainsFromTSCalls(astInfo, allCalls)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isTypeScriptFile returns true when the given path should be analysed via the
// TypeScript pipeline. It checks both the file extension and the configured
// source language so that a project-level "typescript" setting forces routing
// regardless of extension.
func (e *Engine) isTypeScriptFile(path string) bool {
	ext := filepath.Ext(path)
	if ext == ".ts" || ext == ".tsx" {
		return true
	}
	return e.cfg.Source.Language == "typescript"
}

// matchesExtension returns true if the file has an extension accepted by the
// current configuration. When no explicit extensions are configured, defaults
// are applied based on the source language.
func (e *Engine) matchesExtension(path string) bool {
	ext := filepath.Ext(path)

	// If explicit extensions are configured, use those.
	if len(e.cfg.Source.Extensions) > 0 {
		for _, allowed := range e.cfg.Source.Extensions {
			if ext == allowed {
				return true
			}
		}
		return false
	}

	// Default extensions by language.
	switch e.cfg.Source.Language {
	case "typescript":
		return ext == ".ts" || ext == ".tsx"
	default:
		return ext == ".go"
	}
}

// detectLanguage returns the language string for a file path.
func (e *Engine) detectLanguage(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".ts", ".tsx":
		return "typescript"
	default:
		return "go"
	}
}

// buildAnnotation constructs a fingerprint annotation string from patterns
// and intents.
func (e *Engine) buildAnnotation(patterns *types.PatternResult, intents []*types.IntentInfo) string {
	var patternNames []string
	for _, p := range patterns.Patterns {
		patternNames = append(patternNames, string(p.Type))
	}

	var intentNames []string
	for _, i := range intents {
		intentNames = append(intentNames, i.Kind)
	}

	return e.annotator.Annotate("", patternNames, intentNames)
}

// matchesPattern checks if a file path matches the configured glob pattern.
// This is kept as a standalone helper for backward compatibility with callers
// that test pattern matching in isolation.
func (e *Engine) matchesPattern(path string) bool {
	if e.cfg.Source.Pattern == "" {
		return true
	}

	matched, err := filepath.Match(e.cfg.Source.Pattern, filepath.Base(path))
	if err != nil {
		return false
	}

	// Also check TS extensions when the source language is typescript.
	if !matched && e.cfg.Source.Language == "typescript" {
		ext := filepath.Ext(path)
		for _, tsExt := range e.cfg.Source.Extensions {
			if ext == tsExt {
				return true
			}
		}
	}

	return matched
}
