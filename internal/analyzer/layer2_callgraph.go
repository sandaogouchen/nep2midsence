package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// CallGraphAnalyzer builds L2 call chains by expanding function calls found in
// L1 ASTInfo. It performs BFS/DFS walks over function bodies, resolves callees,
// marks nep API calls, and expands local functions recursively up to maxDepth.
type CallGraphAnalyzer struct {
	astInfos map[string]*types.ASTInfo // keyed by file path
	nepAPIs  map[string]bool           // set of known nep API name prefixes
	maxDepth int
}

// NewCallGraphAnalyzer creates a CallGraphAnalyzer.
// maxDepth controls how deep local-function expansion may go.
// nepPrefixes are substrings/prefixes used to recognise nep API calls
// (e.g. "page.", "nep.", "browser.").
func NewCallGraphAnalyzer(maxDepth int, nepPrefixes []string) *CallGraphAnalyzer {
	apis := make(map[string]bool, len(nepPrefixes))
	for _, p := range nepPrefixes {
		apis[p] = true
	}
	return &CallGraphAnalyzer{
		astInfos: make(map[string]*types.ASTInfo),
		nepAPIs:  apis,
		maxDepth: maxDepth,
	}
}

// SetASTInfos supplies the L1 analysis results keyed by file path.
func (a *CallGraphAnalyzer) SetASTInfos(infos map[string]*types.ASTInfo) {
	a.astInfos = infos
}

// BuildAllChains builds call chains for every test function found in the given
// ASTInfo and returns them as a slice.
func (a *CallGraphAnalyzer) BuildAllChains(astInfo *types.ASTInfo) []*types.CallChain {
	var chains []*types.CallChain
	for i := range astInfo.Functions {
		fi := &astInfo.Functions[i]
		if !fi.IsTest {
			continue
		}
		chain := a.BuildCallChain(fi, astInfo.FilePath)
		if chain != nil {
			chains = append(chains, chain)
		}
	}
	return chains
}

// BuildCallChain constructs the full CallChain for a single function by
// parsing its body and recursively expanding local calls.
func (a *CallGraphAnalyzer) BuildCallChain(funcInfo *types.FuncInfo, filePath string) *types.CallChain {
	chain := &types.CallChain{
		EntryFunc: funcInfo.Name,
		MaxDepth:  a.maxDepth,
	}

	body := funcInfo.Body
	if body == "" {
		return chain
	}

	// Parse the function body into an AST so we can walk call expressions.
	stmts := a.parseBody(body)
	if stmts == nil {
		return chain
	}

	// visited tracks functions already expanded to prevent infinite recursion.
	visited := make(map[string]bool)
	a.walkStatements(stmts, funcInfo.Name, filePath, 0, visited, chain)

	// Collect nep API subset.
	for _, s := range chain.Steps {
		if s.IsNepAPI {
			chain.NepAPICalls = append(chain.NepAPICalls, s)
		}
	}
	return chain
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// parseBody wraps the raw function-body source in a function so the standard
// Go parser can handle it, then returns the body statements.
func (a *CallGraphAnalyzer) parseBody(body string) []ast.Stmt {
	// Wrap in a dummy function so it forms a valid Go file.
	src := "package _p\nfunc _f() {\n" + body + "\n}\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.AllErrors)
	if err != nil {
		return nil
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Body != nil {
			return fn.Body.List
		}
	}
	return nil
}

// walkStatements traverses a slice of statements, extracts CallExprs, and
// appends CallSteps to the chain.  Local functions are expanded recursively.
func (a *CallGraphAnalyzer) walkStatements(stmts []ast.Stmt, caller string, filePath string, depth int, visited map[string]bool, chain *types.CallChain) {
	if depth > a.maxDepth {
		return
	}
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			a.processCallExpr(ce, caller, filePath, depth, visited, chain)
			return true
		})
	}
}

// processCallExpr handles a single *ast.CallExpr, creates a CallStep, and
// optionally expands local function bodies.
func (a *CallGraphAnalyzer) processCallExpr(ce *ast.CallExpr, caller string, filePath string, depth int, visited map[string]bool, chain *types.CallChain) {
	callee := a.resolveCallee(ce)
	if callee == "" {
		return
	}

	args, argsRaw := a.extractArgs(ce)
	isNep := a.isNepAPI(callee)
	isLocal := a.isLocalFunc(callee, filePath)

	step := types.CallStep{
		Depth:    depth,
		Caller:   caller,
		Callee:   callee,
		FilePath: filePath,
		Line:     0, // line numbers are relative to the synthetic parse; kept as 0
		Args:     args,
		ArgsRaw:  argsRaw,
		IsNepAPI: isNep,
		IsLocal:  isLocal,
	}
	chain.Steps = append(chain.Steps, step)

	// Expand local functions recursively.
	if isLocal && depth < a.maxDepth && !visited[callee] {
		visited[callee] = true
		if fi := a.lookupFunc(callee, filePath); fi != nil {
			if bodyStmts := a.parseBody(fi.Body); bodyStmts != nil {
				a.walkStatements(bodyStmts, callee, filePath, depth+1, visited, chain)
			}
		}
		visited[callee] = false // allow the same function in different branches
	}
}

// resolveCallee turns a CallExpr's Fun into a human-readable callee name.
// Method calls like page.Click() become "page.Click"; if the receiver is a
// type identifier the result is "TypeName.MethodName".
func (a *CallGraphAnalyzer) resolveCallee(ce *ast.CallExpr) string {
	switch fun := ce.Fun.(type) {
	case *ast.Ident:
		// Simple function call: foo()
		return fun.Name
	case *ast.SelectorExpr:
		// Method / qualified call: x.Foo()
		recv := a.exprString(fun.X)
		return recv + "." + fun.Sel.Name
	case *ast.IndexExpr:
		// Generic instantiation: foo[T]()
		return a.exprString(fun.X)
	case *ast.FuncLit:
		// Immediately invoked func literal – skip callee name
		return "(anon)"
	default:
		return a.exprString(ce.Fun)
	}
}

// isNepAPI checks whether the callee name matches any known nep API prefix.
func (a *CallGraphAnalyzer) isNepAPI(callee string) bool {
	for prefix := range a.nepAPIs {
		if strings.HasPrefix(callee, prefix) {
			return true
		}
	}
	return false
}

// isLocalFunc checks whether the callee (possibly "Receiver.Method") is
// defined in the current file's ASTInfo.
func (a *CallGraphAnalyzer) isLocalFunc(callee string, filePath string) bool {
	return a.lookupFunc(callee, filePath) != nil
}

// lookupFunc searches all loaded ASTInfos (preferring the given filePath) for
// a FuncInfo whose Name or Receiver.Name matches callee.
func (a *CallGraphAnalyzer) lookupFunc(callee string, filePath string) *types.FuncInfo {
	// Determine the simple name and optional receiver from callee.
	simpleName := callee
	receiver := ""
	if dot := strings.LastIndex(callee, "."); dot >= 0 {
		receiver = callee[:dot]
		simpleName = callee[dot+1:]
	}

	// Search the same file first, then fall back to other files.
	order := []string{filePath}
	for p := range a.astInfos {
		if p != filePath {
			order = append(order, p)
		}
	}

	for _, p := range order {
		info, ok := a.astInfos[p]
		if !ok {
			continue
		}
		for i := range info.Functions {
			fi := &info.Functions[i]
			if fi.Name == simpleName && (receiver == "" || fi.Receiver == receiver) {
				return fi
			}
			// Also match Receiver.Name style stored in FuncInfo.
			fullName := fi.Name
			if fi.Receiver != "" {
				fullName = fi.Receiver + "." + fi.Name
			}
			if fullName == callee {
				return fi
			}
		}
	}
	return nil
}

// extractArgs returns two parallel slices: human-friendly arg strings and raw
// source representations of every argument in a call expression.
func (a *CallGraphAnalyzer) extractArgs(ce *ast.CallExpr) (friendly []string, raw []string) {
	for _, arg := range ce.Args {
		s := a.exprString(arg)
		raw = append(raw, s)
		friendly = append(friendly, a.simplifyArg(s))
	}
	return
}

// simplifyArg returns a shorter, human-readable version of an argument string.
func (a *CallGraphAnalyzer) simplifyArg(s string) string {
	// If it is a string literal, strip quotes for readability.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// exprString renders an ast.Expr back to source text.
func (a *CallGraphAnalyzer) exprString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buf strings.Builder
	fset := token.NewFileSet()
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return fmt.Sprintf("%T", expr)
	}
	return buf.String()
}
