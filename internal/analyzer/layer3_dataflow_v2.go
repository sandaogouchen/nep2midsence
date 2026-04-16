// layer3_dataflow_v2.go adds TypeScript support to the L3 data flow layer.
//
// AnalyzeTS performs lightweight data flow analysis for TypeScript files
// using the pre-extracted variable initializers from the TS AST extractor
// rather than walking Go AST ValueSpec nodes.
package analyzer

import (
	"regexp"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// tsTemplateStringRe matches ES6 template literal expressions: ${...}
var tsTemplateStringRe = regexp.MustCompile(`\$\{[^}]+\}`)

// tsDestructureRe matches destructuring patterns: { a, b } or [ a, b ]
var tsDestructureRe = regexp.MustCompile(`^[\[{]`)

// AnalyzeTS performs lightweight data flow analysis for TypeScript files
// using the pre-extracted variable initializers from the TS AST extractor.
//
// It registers constants and variables into the binding table, then resolves
// nep API call arguments against those bindings (same resolution logic as the
// Go path). Complex TypeScript expressions such as destructuring assignments
// and template literals are marked as unresolved.
func (a *DataFlowAnalyzer) AnalyzeTS(astInfo *types.ASTInfo, chains []*types.CallChain) []*types.ValueInfo {
	a.bindings = make(map[string]*types.ValueInfo)

	// Register constants.
	for _, c := range astInfo.Constants {
		a.bindings[c.Name] = &types.ValueInfo{
			Kind:     types.ValueConst,
			Value:    c.Value,
			Source:   "const " + c.Name,
			Variable: c.Name,
			DefinedAt: types.Location{
				File: astInfo.FilePath,
				Line: c.Line,
			},
		}
	}

	// Register variables with initializers.
	for _, v := range astInfo.Variables {
		if v.Value == "" {
			continue
		}

		vi := &types.ValueInfo{
			Kind:     types.ValueLiteral,
			Value:    v.Value,
			Source:   "var " + v.Name,
			Variable: v.Name,
			DefinedAt: types.Location{
				File: astInfo.FilePath,
				Line: v.Line,
			},
		}

		// Mark complex expressions as unresolved so downstream layers
		// know they cannot rely on exact values.
		if isComplexTSExpression(v.Value) {
			vi.Kind = types.ValueUnknown
			vi.Source = "complex_ts_expr " + v.Name
		}

		a.bindings[v.Name] = vi
	}

	// Match bindings to nep API call arguments (same logic as Go path).
	var results []*types.ValueInfo
	for _, chain := range chains {
		for _, step := range chain.NepAPICalls {
			for argIdx, arg := range step.Args {
				if vi := a.resolveTSArg(arg, step, argIdx, astInfo.FilePath); vi != nil {
					results = append(results, vi)
				}
			}
		}
	}

	return results
}

// resolveTSArg attempts to resolve a TypeScript call argument to a known
// value binding.
func (a *DataFlowAnalyzer) resolveTSArg(arg string, step types.CallStep, argIdx int, filePath string) *types.ValueInfo {
	// Check binding table.
	if vi, ok := a.bindings[arg]; ok {
		resolved := *vi
		resolved.ArgIndex = argIdx
		resolved.StepLine = step.Line
		return &resolved
	}

	// Single-quoted or double-quoted string literal.
	if isStringLiteral(arg) {
		return &types.ValueInfo{
			Kind:     types.ValueLiteral,
			Value:    trimQuotes(arg),
			Source:   "literal",
			ArgIndex: argIdx,
			StepLine: step.Line,
			DefinedAt: types.Location{
				File: filePath,
				Line: step.Line,
			},
		}
	}

	// Template literal (backtick string).
	if len(arg) >= 2 && arg[0] == '`' && arg[len(arg)-1] == '`' {
		inner := arg[1 : len(arg)-1]
		kind := types.ValueLiteral
		if tsTemplateStringRe.MatchString(inner) {
			// Contains interpolation \u2014 cannot fully resolve.
			kind = types.ValueUnknown
		}
		return &types.ValueInfo{
			Kind:     kind,
			Value:    inner,
			Source:   "template_literal",
			ArgIndex: argIdx,
			StepLine: step.Line,
			DefinedAt: types.Location{
				File: filePath,
				Line: step.Line,
			},
		}
	}

	// Numeric literal.
	if isNumericLiteral(arg) {
		return &types.ValueInfo{
			Kind:     types.ValueLiteral,
			Value:    arg,
			Source:   "literal",
			ArgIndex: argIdx,
			StepLine: step.Line,
			DefinedAt: types.Location{
				File: filePath,
				Line: step.Line,
			},
		}
	}

	// Boolean literals.
	if arg == "true" || arg == "false" {
		return &types.ValueInfo{
			Kind:     types.ValueLiteral,
			Value:    arg,
			Source:   "literal",
			ArgIndex: argIdx,
			StepLine: step.Line,
			DefinedAt: types.Location{
				File: filePath,
				Line: step.Line,
			},
		}
	}

	// Unresolvable \u2014 return nil.
	return nil
}

// isComplexTSExpression returns true for TypeScript value expressions that
// cannot be statically resolved.
func isComplexTSExpression(value string) bool {
	trimmed := strings.TrimSpace(value)

	if tsDestructureRe.MatchString(trimmed) {
		return true
	}

	if strings.Contains(trimmed, "${") {
		return true
	}

	if strings.HasPrefix(trimmed, "await ") {
		return true
	}

	return false
}

// isStringLiteral checks for single or double quoted strings.
func isStringLiteral(s string) bool {
	if len(s) < 2 {
		return false
	}
	return (s[0] == '"' && s[len(s)-1] == '"') ||
		(s[0] == '\'' && s[len(s)-1] == '\'')
}

// trimQuotes removes the outer quote characters from a string literal.
func trimQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	return s[1 : len(s)-1]
}

// isNumericLiteral does a simple check for numeric values.
func isNumericLiteral(s string) bool {
	if s == "" {
		return false
	}
	dotSeen := false
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	for i := start; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			if dotSeen {
				return false
			}
			dotSeen = true
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
