package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// TSExtractResult is the raw JSON output from the TypeScript extractor script.
type TSExtractResult struct {
	FilePath  string          `json:"filePath"`
	Imports   []tsImportEntry `json:"imports"`
	Functions []tsFuncEntry   `json:"functions"`
	Calls     []tsCallEntry   `json:"calls"`
	Constants []tsConstEntry  `json:"constants"`
	Variables []tsVarEntry    `json:"variables"`
}

type tsImportEntry struct {
	Module string `json:"module"`
	Names  string `json:"names,omitempty"`
	Alias  string `json:"alias,omitempty"`
	Line   int    `json:"line"`
	IsNep  bool   `json:"isNep,omitempty"`
}

type tsFuncEntry struct {
	Name      string   `json:"name"`
	IsTest    bool     `json:"isTest"`
	IsAsync   bool     `json:"isAsync"`
	IsHelper  bool     `json:"isHelper"`
	Params    []string `json:"params,omitempty"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Doc       string   `json:"doc,omitempty"`
	Receiver  string   `json:"receiver,omitempty"`
}

type tsCallEntry struct {
	Receiver      string   `json:"receiver,omitempty"`
	FullReceiver  string   `json:"fullReceiver,omitempty"`
	FuncName      string   `json:"funcName"`
	OwnerRoot     string   `json:"ownerRoot,omitempty"`
	OwnerKind     string   `json:"ownerKind,omitempty"`
	OwnerSource   string   `json:"ownerSource,omitempty"`
	OwnerFile     string   `json:"ownerFile,omitempty"`
	Args          []string `json:"args,omitempty"`
	Line          int      `json:"line"`
	IsNepAPI      bool     `json:"isNepAPI"`
	IsNep         bool     `json:"isNep"`
	IsAwait       bool     `json:"isAwait"`
	IsChained     bool     `json:"isChained"`
	IsWrapperCall bool     `json:"isWrapperCall,omitempty"`
	Callee        string   `json:"callee,omitempty"`
	InFunc        string   `json:"inFunc,omitempty"`
}

type tsConstEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
	Line  int    `json:"line"`
}

type tsVarEntry struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	Type  string `json:"type,omitempty"`
	Line  int    `json:"line"`
}

// TSBridge communicates with the external TypeScript AST extractor script
// via a child Node.js process. It converts the extracted JSON into the
// unified types used by the Go analysis layers L2-L5.
type TSBridge struct {
	nodePath   string
	scriptPath string
	timeout    time.Duration
}

// NewTSBridge creates a TSBridge. nodePath is the path to the node binary,
// scriptPath is the path to the TS extractor script, and timeout is the
// maximum duration for a single extraction invocation.
func NewTSBridge(nodePath, scriptPath string, timeout time.Duration) *TSBridge {
	if nodePath == "" {
		nodePath = "node"
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &TSBridge{
		nodePath:   nodePath,
		scriptPath: scriptPath,
		timeout:    timeout,
	}
}

// Extract invokes the TypeScript extractor on the given file paths and
// returns the raw extraction results. The extractor script is expected to
// accept file paths as CLI arguments and produce a JSON array on stdout.
func (b *TSBridge) Extract(filePaths []string) ([]TSExtractResult, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("tsbridge: no files to extract")
	}

	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	args := append([]string{b.scriptPath}, filePaths...)
	cmd := exec.CommandContext(ctx, b.nodePath, args...)

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("tsbridge: extraction timed out after %v", b.timeout)
		}
		return nil, fmt.Errorf("tsbridge: extraction failed: %w", err)
	}

	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("tsbridge: empty extraction output")
	}

	var results []TSExtractResult
	if trimmed[0] == '{' {
		var single TSExtractResult
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return nil, fmt.Errorf("tsbridge: failed to parse extraction output: %w", err)
		}
		return []TSExtractResult{single}, nil
	}
	if err := json.Unmarshal(trimmed, &results); err != nil {
		return nil, fmt.Errorf("tsbridge: failed to parse extraction output: %w", err)
	}

	return results, nil
}

// ConvertToASTInfo converts a single TSExtractResult into the unified ASTInfo
// representation consumed by layers L2-L5.
func (b *TSBridge) ConvertToASTInfo(r TSExtractResult) *types.ASTInfo {
	info := &types.ASTInfo{
		FilePath: r.FilePath,
		Package:  "", // TypeScript has no package concept; left empty
	}

	// Convert imports
	for _, imp := range r.Imports {
		info.Imports = append(info.Imports, types.ImportInfo{
			Alias: imp.Alias,
			Name:  imp.Names,
			Path:  imp.Module,
			Line:  imp.Line,
			IsNep: imp.IsNep,
		})
	}

	// Convert functions
	for _, fn := range r.Functions {
		fi := types.FuncInfo{
			Name:      fn.Name,
			IsTest:    fn.IsTest,
			IsHelper:  fn.IsHelper,
			LineStart: fn.StartLine,
			LineEnd:   fn.EndLine,
			Doc:       fn.Doc,
			Receiver:  fn.Receiver,
		}
		// Convert string params to ParamInfo
		for _, p := range fn.Params {
			fi.Params = append(fi.Params, types.ParamInfo{Name: p})
		}
		info.Functions = append(info.Functions, fi)
	}

	// Convert constants
	for _, c := range r.Constants {
		info.Constants = append(info.Constants, types.ConstInfo{
			Name:  c.Name,
			Value: c.Value,
			Type:  c.Type,
			Line:  c.Line,
		})
	}

	// Convert variables
	for _, v := range r.Variables {
		info.Variables = append(info.Variables, types.VarInfo{
			Name:  v.Name,
			Value: v.Value,
			Type:  v.Type,
			Line:  v.Line,
		})
	}

	return info
}

// ConvertAllCalls converts all call entries from a TSExtractResult into
// the unified CallStep slice used by L2 chain building.
func (b *TSBridge) ConvertAllCalls(r TSExtractResult, astInfo *types.ASTInfo, cfg *config.Config) []types.CallStep {
	steps := make([]types.CallStep, 0, len(r.Calls))
	ownerCtx := buildTSOwnerContext(r.FilePath, astInfo, cfg)
	for _, c := range r.Calls {
		ownerRoot, ownerKind, ownerSource, ownerFile := ownerCtx.classify(c.Callee, c.FullReceiver, c.FuncName)
		if ownerRoot == "" {
			ownerRoot = c.OwnerRoot
		}
		if ownerKind == "unknown" && c.OwnerKind != "" {
			ownerKind = c.OwnerKind
		}
		if ownerSource == "fallback_unknown" && c.OwnerSource != "" {
			ownerSource = c.OwnerSource
		}
		if ownerFile == "" {
			ownerFile = c.OwnerFile
		}
		steps = append(steps, types.CallStep{
			Receiver:      c.Receiver,
			FullReceiver:  c.FullReceiver,
			FuncName:      c.FuncName,
			OwnerRoot:     ownerRoot,
			OwnerKind:     ownerKind,
			OwnerSource:   ownerSource,
			OwnerFile:     ownerFile,
			Args:          c.Args,
			Line:          c.Line,
			IsNep:         c.IsNep,
			IsNepAPI:      c.IsNepAPI,
			IsAwait:       c.IsAwait,
			IsChained:     c.IsChained,
			IsWrapperCall: ownerKind == "business",
			Callee:        c.Callee,
			InFunc:        c.InFunc,
		})
	}
	return steps
}
