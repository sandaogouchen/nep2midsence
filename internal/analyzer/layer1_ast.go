// Package analyzer provides multi-layer analysis for Go source files.
// layer1_ast.go implements the L1 AST structural analyzer that parses Go source
// files and extracts imports, functions, structs, constants, variables, and init blocks.
package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// ---------------------------------------------------------------------------
// PackageResolver resolves import paths to filesystem directories so that
// local dependencies can be recursively parsed.
// ---------------------------------------------------------------------------

// PackageResolver maps Go import paths to filesystem directories.
type PackageResolver struct {
	// roots maps import-path prefixes to filesystem root directories.
	// For example "github.com/sandaogouchen/nep2midsence" -> "/home/user/project".
	roots map[string]string
}

// NewPackageResolver creates a PackageResolver with the given prefix-to-directory mappings.
func NewPackageResolver(roots map[string]string) *PackageResolver {
	return &PackageResolver{roots: roots}
}

// Resolve attempts to map an import path to a local directory on disk.
// It returns the directory path and true if a mapping was found, or ("", false) otherwise.
func (r *PackageResolver) Resolve(importPath string) (string, bool) {
	if r == nil {
		return "", false
	}
	for prefix, dir := range r.roots {
		if importPath == prefix {
			return dir, true
		}
		if strings.HasPrefix(importPath, prefix+"/") {
			rel := strings.TrimPrefix(importPath, prefix+"/")
			return filepath.Join(dir, rel), true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// ASTAnalyzer – the L1 structural analyser
// ---------------------------------------------------------------------------

// ASTAnalyzer parses Go source files and extracts structural information.
type ASTAnalyzer struct {
	fset        *token.FileSet
	nepPrefixes []string
	parsed      map[string]bool // tracks already-parsed files to avoid circular deps
	resolver    *PackageResolver
}

// NewASTAnalyzer creates an ASTAnalyzer that recognises the given import-path
// prefixes as "nep framework" packages.
func NewASTAnalyzer(nepPrefixes []string) *ASTAnalyzer {
	return &ASTAnalyzer{
		fset:        token.NewFileSet(),
		nepPrefixes: nepPrefixes,
		parsed:      make(map[string]bool),
	}
}

// SetResolver attaches a PackageResolver so that local dependencies discovered
// during analysis can be recursively parsed.
func (a *ASTAnalyzer) SetResolver(r *PackageResolver) {
	a.resolver = r
}

// ---------------------------------------------------------------------------
// Public entry point
// ---------------------------------------------------------------------------

// Analyze parses a single Go source file and returns its structural information.
// If a PackageResolver is configured, local dependencies are parsed recursively
// (guarded against circular imports via the parsed map).
func (a *ASTAnalyzer) Analyze(filePath string) (*types.ASTInfo, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolving absolute path for %s: %w", filePath, err)
	}

	if a.parsed[absPath] {
		// Already parsed – return a minimal stub to break circular references.
		return &types.ASTInfo{FilePath: absPath}, nil
	}
	a.parsed[absPath] = true

	src, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", absPath, err)
	}

	file, err := parser.ParseFile(a.fset, absPath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing file %s: %w", absPath, err)
	}

	info := &types.ASTInfo{
		FilePath: absPath,
		Package:  file.Name.Name,
	}

	info.Imports = a.extractImports(file)
	info.Functions = a.extractFunctions(file, src)
	info.Structs = a.extractStructs(file)
	info.Constants = a.extractConstants(file, src)
	info.Variables = a.extractVariables(file, src)
	info.InitBlocks = a.extractInitBlocks(file, src)

	return info, nil
}

// AnalyzeRecursive behaves like Analyze but also recursively parses every
// local dependency that can be resolved through the PackageResolver.
// It returns ASTInfo results keyed by absolute file path.
func (a *ASTAnalyzer) AnalyzeRecursive(filePath string) (map[string]*types.ASTInfo, error) {
	results := make(map[string]*types.ASTInfo)
	if err := a.analyzeRecursiveInner(filePath, results); err != nil {
		return nil, err
	}
	return results, nil
}

func (a *ASTAnalyzer) analyzeRecursiveInner(filePath string, results map[string]*types.ASTInfo) error {
	info, err := a.Analyze(filePath)
	if err != nil {
		return err
	}
	results[info.FilePath] = info

	// Resolve and recurse into local imports.
	for _, imp := range info.Imports {
		dir, ok := a.resolver.Resolve(imp.Path)
		if !ok {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // non-fatal – the import may not exist locally
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			// Skip test files when following dependencies.
			if strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			childPath := filepath.Join(dir, entry.Name())
			absChild, err := filepath.Abs(childPath)
			if err != nil {
				continue
			}
			if a.parsed[absChild] {
				continue
			}
			if err := a.analyzeRecursiveInner(childPath, results); err != nil {
				// Log but do not abort – best-effort recursion.
				continue
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Import extraction
// ---------------------------------------------------------------------------

func (a *ASTAnalyzer) extractImports(file *ast.File) []types.ImportInfo {
	var imports []types.ImportInfo
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		imports = append(imports, types.ImportInfo{
			Path:  path,
			Alias: alias,
			IsNep: a.isNepPackage(path),
		})
	}
	return imports
}

// isNepPackage reports whether the given import path belongs to the nep
// framework (matched by any of the configured prefixes).
func (a *ASTAnalyzer) isNepPackage(path string) bool {
	for _, prefix := range a.nepPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Function extraction
// ---------------------------------------------------------------------------

func (a *ASTAnalyzer) extractFunctions(file *ast.File, src []byte) []types.FuncInfo {
	var funcs []types.FuncInfo
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok {
			funcs = append(funcs, a.buildFuncInfo(fn, src))
		}
	}
	return funcs
}

func (a *ASTAnalyzer) buildFuncInfo(fn *ast.FuncDecl, src []byte) types.FuncInfo {
	info := types.FuncInfo{
		Name:      fn.Name.Name,
		Doc:       extractDoc(fn.Doc),
		Params:    extractParams(fn.Type.Params),
		Results:   extractParams(fn.Type.Results),
		Receiver:  extractReceiver(fn.Recv),
		LineStart: a.fset.Position(fn.Pos()).Line,
		LineEnd:   a.fset.Position(fn.End()).Line,
		Body:      a.nodeSource(fn.Body, src),
	}

	info.IsTest = isTestFunc(info.Name, info.Params)
	info.IsHelper = isHelperFunc(info.Name, info.Params)
	if fn.Body != nil {
		info.SubTests = extractSubTests(fn.Body)
	}

	return info
}

// isTestFunc returns true when the function looks like a Go test function:
// name starts with "Test" and first param is *testing.T (or *testing.B / *testing.M).
func isTestFunc(name string, params []types.ParamInfo) bool {
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	if len(params) == 0 {
		return false
	}
	first := params[0].Type
	return first == "*testing.T" || first == "*testing.B" || first == "*testing.M"
}

// isHelperFunc detects common helper patterns: functions receiving *testing.T
// whose name does NOT start with "Test".
func isHelperFunc(name string, params []types.ParamInfo) bool {
	if strings.HasPrefix(name, "Test") {
		return false
	}
	for _, p := range params {
		if p.Type == "*testing.T" || p.Type == "*testing.B" {
			return true
		}
	}
	return false
}

// extractSubTests walks the function body looking for t.Run("name", ...) calls
// and returns the list of sub-test names found.
func extractSubTests(body *ast.BlockStmt) []string {
	var names []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" {
			return true
		}
		// The receiver must be an identifier (typically "t" or "b").
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		_ = ident // we don't constrain the variable name

		if len(call.Args) >= 1 {
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				names = append(names, strings.Trim(lit.Value, `"`))
			}
		}
		return true
	})
	return names
}

// ---------------------------------------------------------------------------
// Struct extraction
// ---------------------------------------------------------------------------

func (a *ASTAnalyzer) extractStructs(file *ast.File) []types.StructInfo {
	var structs []types.StructInfo
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			si := types.StructInfo{
				Name:   ts.Name.Name,
				Doc:    extractDoc(gd.Doc),
				Fields: extractFields(st),
			}
			// Collect method names declared with this struct as receiver.
			si.Methods = collectMethods(file, si.Name)
			structs = append(structs, si)
		}
	}
	return structs
}

func extractFields(st *ast.StructType) []types.FieldInfo {
	var fields []types.FieldInfo
	if st.Fields == nil {
		return fields
	}
	for _, f := range st.Fields.List {
		typeName := exprToString(f.Type)
		tag := ""
		if f.Tag != nil {
			tag = strings.Trim(f.Tag.Value, "`")
		}

		if len(f.Names) == 0 {
			// Embedded field.
			fields = append(fields, types.FieldInfo{
				Name: typeName,
				Type: typeName,
				Tag:  tag,
			})
			continue
		}
		for _, name := range f.Names {
			fields = append(fields, types.FieldInfo{
				Name: name.Name,
				Type: typeName,
				Tag:  tag,
			})
		}
	}
	return fields
}

// collectMethods returns the names of methods whose receiver type matches
// the given struct name (pointer or value receiver).
func collectMethods(file *ast.File, structName string) []string {
	var methods []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		recvType := extractReceiverTypeName(fn.Recv.List[0].Type)
		if recvType == structName {
			methods = append(methods, fn.Name.Name)
		}
	}
	return methods
}

// ---------------------------------------------------------------------------
// Constant extraction
// ---------------------------------------------------------------------------

func (a *ASTAnalyzer) extractConstants(file *ast.File, src []byte) []types.ConstInfo {
	var consts []types.ConstInfo
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typeName := ""
			if vs.Type != nil {
				typeName = exprToString(vs.Type)
			}
			for i, name := range vs.Names {
				val := ""
				if i < len(vs.Values) {
					val = a.nodeSource(vs.Values[i], src)
				}
				consts = append(consts, types.ConstInfo{
					Name:  name.Name,
					Value: val,
					Type:  typeName,
				})
			}
		}
	}
	return consts
}

// ---------------------------------------------------------------------------
// Variable extraction
// ---------------------------------------------------------------------------

func (a *ASTAnalyzer) extractVariables(file *ast.File, src []byte) []types.VarInfo {
	var vars []types.VarInfo
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typeName := ""
			if vs.Type != nil {
				typeName = exprToString(vs.Type)
			}
			for i, name := range vs.Names {
				val := ""
				if i < len(vs.Values) {
					val = a.nodeSource(vs.Values[i], src)
				}
				vars = append(vars, types.VarInfo{
					Name:  name.Name,
					Type:  typeName,
					Value: val,
				})
			}
		}
	}
	return vars
}

// ---------------------------------------------------------------------------
// Init block extraction
// ---------------------------------------------------------------------------

func (a *ASTAnalyzer) extractInitBlocks(file *ast.File, src []byte) []types.InitInfo {
	var inits []types.InitInfo
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		kind := ""
		switch fn.Name.Name {
		case "init":
			kind = "init"
		case "TestMain":
			kind = "TestMain"
		default:
			continue
		}
		inits = append(inits, types.InitInfo{
			Kind:      kind,
			LineStart: a.fset.Position(fn.Pos()).Line,
			LineEnd:   a.fset.Position(fn.End()).Line,
			Body:      a.nodeSource(fn.Body, src),
		})
	}
	return inits
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// extractDoc returns the text of a *ast.CommentGroup, or "" if nil.
func extractDoc(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return strings.TrimSpace(cg.Text())
}

// extractParams converts an *ast.FieldList to a slice of ParamInfo.
func extractParams(fl *ast.FieldList) []types.ParamInfo {
	if fl == nil {
		return nil
	}
	var params []types.ParamInfo
	for _, f := range fl.List {
		typeName := exprToString(f.Type)
		if len(f.Names) == 0 {
			params = append(params, types.ParamInfo{Type: typeName})
			continue
		}
		for _, name := range f.Names {
			params = append(params, types.ParamInfo{
				Name: name.Name,
				Type: typeName,
			})
		}
	}
	return params
}

// extractReceiver returns the receiver type string (e.g. "*MyStruct") for a
// method, or "" for a plain function.
func extractReceiver(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	return exprToString(fl.List[0].Type)
}

// extractReceiverTypeName returns the base type name from a receiver expression,
// stripping any pointer indirection. Used to match methods to structs.
func extractReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return extractReceiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		// Generic receiver like T[Elem].
		return extractReceiverTypeName(t.X)
	default:
		return ""
	}
}

// exprToString produces a human-readable Go source representation of an AST
// expression. It uses go/printer for accuracy and falls back to a simple
// recursive stringifier for the most common node kinds.
func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buf strings.Builder
	cfg := printer.Config{Mode: printer.RawFormat}
	if err := cfg.Fprint(&buf, token.NewFileSet(), expr); err == nil && buf.Len() > 0 {
		return buf.String()
	}
	// Fallback for cases where printer cannot handle a detached node.
	return exprToStringFallback(expr)
}

func exprToStringFallback(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToStringFallback(t.X)
	case *ast.SelectorExpr:
		return exprToStringFallback(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + exprToStringFallback(t.Elt)
		}
		return "[" + exprToStringFallback(t.Len) + "]" + exprToStringFallback(t.Elt)
	case *ast.MapType:
		return "map[" + exprToStringFallback(t.Key) + "]" + exprToStringFallback(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + exprToStringFallback(t.Elt)
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + exprToStringFallback(t.Value)
	case *ast.BasicLit:
		return t.Value
	default:
		return fmt.Sprintf("%T", expr)
	}
}

// nodeSource extracts the source text corresponding to an AST node directly
// from the original source bytes. Returns "" for nil nodes.
func (a *ASTAnalyzer) nodeSource(node ast.Node, src []byte) string {
	if node == nil {
		return ""
	}
	start := a.fset.Position(node.Pos()).Offset
	end := a.fset.Position(node.End()).Offset
	if start < 0 || end < 0 || start >= len(src) || end > len(src) || start >= end {
		return ""
	}
	return string(src[start:end])
}
