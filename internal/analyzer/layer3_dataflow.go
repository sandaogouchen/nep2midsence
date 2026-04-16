package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// DataFlowAnalyzer tracks selectors, URLs, and test data through variable
// assignments so that each value flowing into a nep API call can be resolved
// back to its origin (literal, constant, concatenation, parameter, …).
type DataFlowAnalyzer struct {
	bindings map[string]*types.ValueInfo
}

// NewDataFlowAnalyzer returns an initialised DataFlowAnalyzer.
func NewDataFlowAnalyzer() *DataFlowAnalyzer {
	return &DataFlowAnalyzer{
		bindings: make(map[string]*types.ValueInfo),
	}
}

// Analyze walks every function body in astInfo, resolves variable assignments,
// and returns ValueInfo entries for each value that flows into a nep API call.
func (a *DataFlowAnalyzer) Analyze(astInfo *types.ASTInfo, chains []*types.CallChain) []*types.ValueInfo {
	// Reset bindings for each analysis run.
	a.bindings = make(map[string]*types.ValueInfo)

	// 1. Register file-level constants.
	for _, c := range astInfo.Constants {
		a.bindings[c.Name] = &types.ValueInfo{
			Kind:     types.ValueConst,
			Value:    c.Value,
			Source:   "const " + c.Name,
			Variable: c.Name,
			DefinedAt: types.Location{
				File: astInfo.FilePath,
			},
		}
	}

	// 2. Register file-level variables that have an initial value.
	for _, v := range astInfo.Variables {
		if v.Value != "" {
			a.bindings[v.Name] = &types.ValueInfo{
				Kind:     types.ValueLiteral,
				Value:    v.Value,
				Source:   "var " + v.Name,
				Variable: v.Name,
				DefinedAt: types.Location{
					File: astInfo.FilePath,
				},
			}
		}
	}

	// 3. Walk each function body to discover local assignments.
	for _, fn := range astInfo.Functions {
		a.walkFunctionBody(fn, astInfo.FilePath)
	}

	// 4. Build a set of nep API callee names for quick lookup.
	nepCallees := make(map[string]bool)
	for _, chain := range chains {
		for _, step := range chain.NepAPICalls {
			nepCallees[step.Callee] = true
		}
	}

	// 5. Match bindings to nep API call arguments.
	var results []*types.ValueInfo
	for _, chain := range chains {
		for _, step := range chain.NepAPICalls {
			for argIdx, arg := range step.Args {
				arg = strings.TrimSpace(arg)
				if vi := a.resolveArg(arg, step, argIdx, astInfo.FilePath); vi != nil {
					results = append(results, vi)
				}
			}
		}
	}

	return results
}

// walkFunctionBody parses and walks a single function body to extract
// variable assignments and short variable declarations.
func (a *DataFlowAnalyzer) walkFunctionBody(fn types.FuncInfo, filePath string) {
	if fn.Body == "" {
		return
	}

	// Register function parameters as ValueParam.
	for _, p := range fn.Params {
		a.bindings[p.Name] = &types.ValueInfo{
			Kind:     types.ValueParam,
			Value:    "",
			Source:   "param " + p.Name + " of " + fn.Name,
			Variable: p.Name,
			DefinedAt: types.Location{
				File: filePath,
				Line: fn.LineStart,
			},
		}
	}

	// Wrap the body so it can be parsed as a complete Go file.
	src := "package _tmp\nfunc _() {\n" + fn.Body + "\n}"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.AllErrors)
	if err != nil {
		return
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			a.handleAssign(stmt, fset, filePath, fn.Name)
		}
		return true
	})
}

// handleAssign processes a single assignment statement and records bindings.
func (a *DataFlowAnalyzer) handleAssign(stmt *ast.AssignStmt, fset *token.FileSet, filePath, funcName string) {
	for i, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || i >= len(stmt.Rhs) {
			continue
		}
		rhs := stmt.Rhs[i]
		line := fset.Position(stmt.Pos()).Line

		vi := a.resolveExpr(rhs, filePath, line, funcName)
		vi.Variable = ident.Name
		a.bindings[ident.Name] = vi
	}
}

// resolveExpr determines the ValueInfo for an arbitrary expression on the
// right-hand side of an assignment.
func (a *DataFlowAnalyzer) resolveExpr(expr ast.Expr, filePath string, line int, funcName string) *types.ValueInfo {
	loc := types.Location{File: filePath, Line: line}

	switch e := expr.(type) {
	case *ast.BasicLit:
		// String or numeric literal.
		val := strings.Trim(e.Value, "\"` ")
		return &types.ValueInfo{
			Kind:      types.ValueLiteral,
			Value:     val,
			Source:    "literal in " + funcName,
			DefinedAt: loc,
		}

	case *ast.Ident:
		// Reference to another variable or constant.
		if existing, ok := a.bindings[e.Name]; ok {
			return copyValueInfo(existing, loc)
		}
		return &types.ValueInfo{
			Kind:      types.ValueUnresolved,
			Value:     e.Name,
			Source:    "ident " + e.Name,
			DefinedAt: loc,
		}

	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			left := a.resolveExpr(e.X, filePath, line, funcName)
			right := a.resolveExpr(e.Y, filePath, line, funcName)
			return &types.ValueInfo{
				Kind:       types.ValueConcatenation,
				Value:      left.Value + right.Value,
				Source:     "concatenation in " + funcName,
				DefinedAt:  loc,
				Components: []types.ValueInfo{*left, *right},
			}
		}

	case *ast.CallExpr:
		calleeName := callExprName(e)
		return &types.ValueInfo{
			Kind:      types.ValueFuncReturn,
			Value:     "",
			Source:    "return of " + calleeName + " in " + funcName,
			DefinedAt: loc,
		}

	case *ast.SelectorExpr:
		name := selectorExprString(e)
		if existing, ok := a.bindings[name]; ok {
			return copyValueInfo(existing, loc)
		}
		return &types.ValueInfo{
			Kind:      types.ValueUnresolved,
			Value:     name,
			Source:    "selector " + name,
			DefinedAt: loc,
		}
	}

	return &types.ValueInfo{
		Kind:      types.ValueUnresolved,
		Value:     "",
		Source:    "unknown expression in " + funcName,
		DefinedAt: types.Location{File: filePath, Line: line},
	}
}

// resolveArg resolves a single argument string to a ValueInfo, attaching
// the appropriate Usage record.
func (a *DataFlowAnalyzer) resolveArg(arg string, step types.CallStep, argIdx int, filePath string) *types.ValueInfo {
	usage := types.Usage{
		NepAPI:   step.Callee,
		ArgIndex: argIdx,
		Location: types.Location{
			File: filePath,
			Line: step.Line,
		},
	}

	// Direct string literal (quoted).
	if isQuoted(arg) {
		val := strings.Trim(arg, "\"'`")
		return &types.ValueInfo{
			Kind:      types.ValueLiteral,
			Value:     val,
			Source:    "literal arg to " + step.Callee,
			Variable:  "",
			DefinedAt: types.Location{File: filePath, Line: step.Line},
			UsedBy:    []types.Usage{usage},
		}
	}

	// Known binding.
	if vi, ok := a.bindings[arg]; ok {
		result := copyValueInfo(vi, vi.DefinedAt)
		result.UsedBy = append(result.UsedBy, usage)
		return result
	}

	// Concatenation expression in argument.
	if strings.Contains(arg, "+") {
		parts := strings.Split(arg, "+")
		var components []types.ValueInfo
		var combined strings.Builder
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if isQuoted(p) {
				val := strings.Trim(p, "\"'`")
				combined.WriteString(val)
				components = append(components, types.ValueInfo{
					Kind:  types.ValueLiteral,
					Value: val,
				})
			} else if vi, ok := a.bindings[p]; ok {
				combined.WriteString(vi.Value)
				components = append(components, *vi)
			} else {
				components = append(components, types.ValueInfo{
					Kind:  types.ValueUnresolved,
					Value: p,
				})
			}
		}
		return &types.ValueInfo{
			Kind:       types.ValueConcatenation,
			Value:      combined.String(),
			Source:     "concatenation arg to " + step.Callee,
			DefinedAt:  types.Location{File: filePath, Line: step.Line},
			UsedBy:     []types.Usage{usage},
			Components: components,
		}
	}

	// Unresolved.
	return &types.ValueInfo{
		Kind:      types.ValueUnresolved,
		Value:     arg,
		Source:    "unresolved arg to " + step.Callee,
		Variable:  arg,
		DefinedAt: types.Location{File: filePath, Line: step.Line},
		UsedBy:    []types.Usage{usage},
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func isQuoted(s string) bool {
	if len(s) < 2 {
		return false
	}
	return (s[0] == '"' && s[len(s)-1] == '"') ||
		(s[0] == '\'' && s[len(s)-1] == '\'') ||
		(s[0] == '`' && s[len(s)-1] == '`')
}

func callExprName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return selectorExprString(fn)
	}
	return "<unknown>"
}

func selectorExprString(sel *ast.SelectorExpr) string {
	if ident, ok := sel.X.(*ast.Ident); ok {
		return ident.Name + "." + sel.Sel.Name
	}
	return sel.Sel.Name
}

func copyValueInfo(src *types.ValueInfo, loc types.Location) *types.ValueInfo {
	vi := *src
	vi.DefinedAt = loc
	// Preserve existing UsedBy entries; callers append new ones.
	usedBy := make([]types.Usage, len(src.UsedBy))
	copy(usedBy, src.UsedBy)
	vi.UsedBy = usedBy
	return &vi
}
