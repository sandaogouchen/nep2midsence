package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/types"
)

var (
	tsImportFromRe   = regexp.MustCompile(`^\s*import\s+(.+?)\s+from\s+['"]([^'"]+)['"]`)
	tsImportBareRe   = regexp.MustCompile(`^\s*import\s+['"]([^'"]+)['"]`)
	tsVarDeclRe      = regexp.MustCompile(`^\s*(?:export\s+)?(const|let|var)\s+([A-Za-z_$][\w$]*)[^=]*=\s*(.+?)\s*;?\s*$`)
	tsFnDeclRe       = regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(`)
	tsArrowFnDeclRe  = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?\([^)]*\)\s*=>`)
	tsTestCallRe     = regexp.MustCompile(`\b(it|test)\s*\(\s*['"` + "`" + `]([^'"` + "`" + `]+)['"` + "`" + `]\s*,`)
	tsTestCallBareRe = regexp.MustCompile(`^\s*(?:commonIt|it|test)\s*\(\s*$`)
	tsTestNameRe     = regexp.MustCompile(`^\s*['"` + "`" + `]([^'"` + "`" + `]+)['"` + "`" + `]\s*,`)
	tsHookCallRe     = regexp.MustCompile(`\b(beforeEach|afterEach|beforeAll|afterAll)\s*\(`)
	// Capture multi-level property access chains like:
	//   adGroupPage.optimizationAndBiddingModule1MNBA.vv_setStandardBtn(
	// Group 1: optional receiver chain with trailing dot ("adGroupPage.optimization...")
	// Group 2: terminal function/method name ("vv_setStandardBtn")
	tsCallRe = regexp.MustCompile(`((?:[A-Za-z_$][\w$]*\.)+)?([A-Za-z_$][\w$]*)\s*\(`)
	// component prompt extraction
	tsClassDeclRe       = regexp.MustCompile(`\bclass\s+([A-Za-z_$][\w$]*)`)
	classExtendsRe      = regexp.MustCompile(`\bclass\s+([A-Za-z_$][\w$]*)\s+extends\s+([A-Za-z_$][\w$]*)`)
	tsDefaultPromptHint = "DEFAULT_PROMPT"
)

// ExtractDefaultPrompts extracts NEP component DEFAULT_PROMPT strings from a TS/JS file.
// It is intentionally heuristic and line-based to work in both TSBridge and fallback modes.
func ExtractDefaultPrompts(filePath string) []types.DefaultPromptInfo {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")

	currentClass := ""
	var out []types.DefaultPromptInfo

	for i, line := range lines {
		// Track the nearest preceding class declaration.
		if m := tsClassDeclRe.FindStringSubmatch(line); len(m) == 2 {
			currentClass = strings.TrimSpace(m[1])
		}

		if !strings.Contains(line, tsDefaultPromptHint) {
			continue
		}
		// Only handle the common static assignment form.
		if !strings.Contains(line, "static") || !strings.Contains(line, "=") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		rhs := strings.TrimSpace(line[idx+1:])
		rhs = strings.TrimSuffix(rhs, ";")
		rhs = strings.TrimSpace(rhs)
		if rhs == "" {
			continue
		}
		quote := rhs[0]
		if quote != '\'' && quote != '"' && quote != '`' {
			continue
		}
		end := strings.LastIndexByte(rhs[1:], quote)
		if end < 0 {
			continue
		}
		val := rhs[1 : 1+end]
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		out = append(out, types.DefaultPromptInfo{
			ClassName:   currentClass,
			PromptValue: val,
			FilePath:    filePath,
			Line:        i + 1,
		})
	}

	return out
}

var tsNewInstanceRe = regexp.MustCompile(`\bnew\s+([A-Za-z_$][\w$]*)\b`)

var obviousInfraSegments = map[string]struct{}{
	"logger":         {},
	"console":        {},
	"json":           {},
	"math":           {},
	"promise":        {},
	"process":        {},
	"sessionstorage": {},
	"localstorage":   {},
}

var promptSafetyInfraMethods = map[string]struct{}{
	"log": {}, "info": {}, "warn": {}, "error": {}, "debug": {},
	"stringify": {}, "parse": {},
	"expect": {}, "assert": {},
}

var tsNepAPIs = map[string]struct{}{
	"navigate": {}, "goto": {}, "navigateTo": {},
	"click": {}, "dblclick": {}, "clickLogin": {}, "clickCreateCampaign": {},
	"sendKeys": {}, "type": {}, "fill": {},
	"fillUsername": {}, "fillPassword": {}, "setBudget": {}, "selectObjective": {},
	"findElement": {}, "querySelector": {},
	"findElements": {}, "querySelectorAll": {},
	"waitForElement": {}, "waitForSelector": {},
	"waitForNavigation": {}, "waitForTimeout": {},
	"assertVisible": {}, "assertText": {}, "assertExists": {},
	"getText": {}, "getInnerText": {}, "getAttribute": {},
	"create": {}, "close": {}, "login": {},
}

func extractTypeScriptFallback(filePath string, cfg *config.Config) (*types.ASTInfo, []types.CallStep, string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("read file: %w", err)
	}

	source := string(content)
	lines := strings.Split(source, "\n")
	astInfo := &types.ASTInfo{FilePath: filePath}
	language := tsLanguageForPath(filePath)

	testRanges := collectTSTestRanges(lines)
	hookRanges := collectTSRanges(lines, tsHookCallRe)

	for _, rng := range testRanges {
		astInfo.Functions = append(astInfo.Functions, types.FuncInfo{
			Name:      rng.name,
			IsTest:    true,
			LineStart: rng.startLine,
			LineEnd:   rng.endLine,
			Body:      strings.Join(lines[rng.startLine-1:rng.endLine], "\n"),
		})
	}
	for _, rng := range hookRanges {
		astInfo.Functions = append(astInfo.Functions, types.FuncInfo{
			Name:      rng.name,
			IsHelper:  true,
			LineStart: rng.startLine,
			LineEnd:   rng.endLine,
			Body:      strings.Join(lines[rng.startLine-1:rng.endLine], "\n"),
		})
	}

	for idx, line := range lines {
		lineNo := idx + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		if matches := tsImportFromRe.FindStringSubmatch(line); len(matches) == 3 {
			astInfo.Imports = append(astInfo.Imports, types.ImportInfo{
				Path:  matches[2],
				Name:  strings.TrimSpace(matches[1]),
				Alias: strings.TrimSpace(matches[1]),
				Line:  lineNo,
				IsNep: strings.Contains(strings.ToLower(matches[2]), "nep"),
			})
		} else if matches := tsImportBareRe.FindStringSubmatch(line); len(matches) == 2 {
			astInfo.Imports = append(astInfo.Imports, types.ImportInfo{
				Path:  matches[1],
				Line:  lineNo,
				IsNep: strings.Contains(strings.ToLower(matches[1]), "nep"),
			})
		}

		if matches := tsVarDeclRe.FindStringSubmatch(line); len(matches) == 4 {
			name := matches[2]
			value := strings.TrimSpace(matches[3])
			if matches[1] == "const" {
				astInfo.Constants = append(astInfo.Constants, types.ConstInfo{Name: name, Value: value, Line: lineNo})
			} else {
				astInfo.Variables = append(astInfo.Variables, types.VarInfo{Name: name, Value: value, Line: lineNo})
			}
		}

		if matches := tsFnDeclRe.FindStringSubmatch(line); len(matches) == 2 {
			endLine := findTSBlockEnd(lines, idx)
			astInfo.Functions = append(astInfo.Functions, types.FuncInfo{
				Name:      matches[1],
				IsHelper:  true,
				LineStart: lineNo,
				LineEnd:   endLine,
				Body:      strings.Join(lines[idx:endLine], "\n"),
			})
		} else if matches := tsArrowFnDeclRe.FindStringSubmatch(line); len(matches) == 2 {
			endLine := findTSBlockEnd(lines, idx)
			astInfo.Functions = append(astInfo.Functions, types.FuncInfo{
				Name:      matches[1],
				IsHelper:  true,
				LineStart: lineNo,
				LineEnd:   endLine,
				Body:      strings.Join(lines[idx:endLine], "\n"),
			})
		}
	}

	if matches := classExtendsRe.FindStringSubmatch(source); len(matches) == 3 {
		astInfo.ClassName = strings.TrimSpace(matches[1])
		astInfo.ExtendsFrom = strings.TrimSpace(matches[2])
		astInfo.ExtendsImport = findImportedSymbolPath(astInfo.Imports, astInfo.ExtendsFrom)
	} else if matches := tsClassDeclRe.FindStringSubmatch(source); len(matches) == 2 {
		astInfo.ClassName = strings.TrimSpace(matches[1])
	}

	allCalls := collectTSCalls(filePath, lines, astInfo, cfg, testRanges, hookRanges)
	return astInfo, allCalls, language, nil
}

func findImportedSymbolPath(imports []types.ImportInfo, symbol string) string {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return ""
	}
	for _, imp := range imports {
		if importSpecIncludesSymbol(imp.Name, symbol) {
			return imp.Path
		}
	}
	return ""
}

func importSpecIncludesSymbol(importSpec string, symbol string) bool {
	importSpec = strings.TrimSpace(importSpec)
	symbol = strings.TrimSpace(symbol)
	if importSpec == "" || symbol == "" {
		return false
	}

	if !strings.HasPrefix(importSpec, "{") {
		fields := strings.Fields(importSpec)
		if len(fields) > 0 && fields[0] == symbol {
			return true
		}
		if strings.Contains(importSpec, ",") {
			parts := strings.Split(importSpec, ",")
			if len(parts) > 0 && strings.TrimSpace(parts[0]) == symbol {
				return true
			}
		}
	}

	if strings.Contains(importSpec, "{") {
		start := strings.Index(importSpec, "{")
		end := strings.LastIndex(importSpec, "}")
		if start >= 0 && end > start {
			items := strings.Split(importSpec[start+1:end], ",")
			for _, item := range items {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				parts := strings.Fields(item)
				if len(parts) == 1 && parts[0] == symbol {
					return true
				}
				if len(parts) >= 3 && (parts[0] == symbol || parts[len(parts)-1] == symbol) {
					return true
				}
			}
		}
	}

	return false
}

type tsRange struct {
	name      string
	startLine int
	endLine   int
}

func collectTSRanges(lines []string, re *regexp.Regexp) []tsRange {
	var ranges []tsRange
	for idx, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 0 {
			continue
		}
		name := matches[1]
		if len(matches) >= 3 {
			name = matches[len(matches)-1]
		}
		ranges = append(ranges, tsRange{
			name:      name,
			startLine: idx + 1,
			endLine:   findTSBlockEnd(lines, idx),
		})
	}
	return ranges
}

// collectTSTestRanges detects it()/test() calls including multi-line patterns
// where the call and the test name string are on separate lines, e.g.:
//
//	it(
//	  'test name',
//	  async () => { ... }
//	)
func collectTSTestRanges(lines []string) []tsRange {
	// First collect single-line matches using the standard regex.
	ranges := collectTSRanges(lines, tsTestCallRe)
	singleLineStarts := make(map[int]struct{}, len(ranges))
	for _, r := range ranges {
		singleLineStarts[r.startLine] = struct{}{}
	}

	// Then scan for multi-line bare it(/test( patterns.
	for idx, line := range lines {
		startLine := idx + 1
		if _, ok := singleLineStarts[startLine]; ok {
			continue // already matched as single-line
		}
		if !tsTestCallBareRe.MatchString(line) {
			continue
		}
		// Look ahead up to 3 lines for the test name string.
		name := ""
		for lookahead := 1; lookahead <= 3 && idx+lookahead < len(lines); lookahead++ {
			nextLine := lines[idx+lookahead]
			if nameMatch := tsTestNameRe.FindStringSubmatch(nextLine); len(nameMatch) >= 2 {
				name = nameMatch[1]
				break
			}
			// Skip blank/comment lines during lookahead.
			trimmed := strings.TrimSpace(nextLine)
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				break
			}
		}
		if name == "" {
			// Could not find a name; still treat as a test block so it gets migrated.
			name = fmt.Sprintf("anonymous_test_L%d", startLine)
		}
		ranges = append(ranges, tsRange{
			name:      name,
			startLine: startLine,
			endLine:   findTSBlockEnd(lines, idx),
		})
	}
	return ranges
}

func findTSBlockEnd(lines []string, startIdx int) int {
	braceDepth := 0
	foundBrace := false

	for idx := startIdx; idx < len(lines); idx++ {
		for _, r := range lines[idx] {
			switch r {
			case '{':
				braceDepth++
				foundBrace = true
			case '}':
				if braceDepth > 0 {
					braceDepth--
				}
				if foundBrace && braceDepth == 0 {
					return idx + 1
				}
			}
		}
	}

	return startIdx + 1
}

func collectTSCalls(filePath string, lines []string, astInfo *types.ASTInfo, cfg *config.Config, ranges ...[]tsRange) []types.CallStep {
	var allRanges []tsRange
	for _, group := range ranges {
		allRanges = append(allRanges, group...)
	}
	ownerCtx := buildTSOwnerContext(filePath, astInfo, cfg)

	var steps []types.CallStep
	for idx, line := range lines {
		lineNo := idx + 1
		matches := tsCallRe.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			if len(match) < 6 {
				continue
			}
			fullCall := line[match[0]:match[1]]
			if shouldSkipTSCall(fullCall) {
				continue
			}

			receiver := ""
			fullReceiver := ""
			if match[2] != -1 && match[3] != -1 {
				fullReceiver = strings.TrimSuffix(line[match[2]:match[3]], ".")
				if fullReceiver != "" {
					parts := strings.Split(fullReceiver, ".")
					receiver = strings.TrimSpace(parts[len(parts)-1])
				}
			}
			funcName := line[match[4]:match[5]]
			args, endIdx := extractTSCallArguments(line, match[1]-1)
			callee := funcName
			if fullReceiver != "" {
				callee = fullReceiver + "." + funcName
			}

			_, isNepAPI := tsNepAPIs[funcName]
			ownerRoot, ownerKind, ownerSource, ownerFile := ownerCtx.classify(callee, fullReceiver, funcName)
			steps = append(steps, types.CallStep{
				Callee:        callee,
				Receiver:      receiver,
				FullReceiver:  fullReceiver,
				FuncName:      funcName,
				Args:          args,
				FilePath:      filePath,
				Line:          lineNo,
				IsNepAPI:      isNepAPI,
				IsNep:         isNepAPI,
				IsAwait:       strings.Contains(line[:match[0]], "await "),
				IsChained:     receiver != "",
				IsWrapperCall: ownerKind == "business",
				OwnerRoot:     ownerRoot,
				OwnerKind:     ownerKind,
				OwnerSource:   ownerSource,
				OwnerFile:     ownerFile,
				InFunc:        enclosingTSRangeName(lineNo, allRanges),
				Comment:       strings.TrimSpace(line[endIdx:]),
			})
		}
	}

	return steps
}

func shouldSkipTSCall(call string) bool {
	switch {
	case strings.HasPrefix(call, "if("), strings.HasPrefix(call, "if ("), strings.HasPrefix(call, "for("), strings.HasPrefix(call, "for "):
		return true
	case strings.HasPrefix(call, "switch("), strings.HasPrefix(call, "switch "):
		return true
	default:
		return false
	}
}

type tsOwnerContext struct {
	filePath         string
	cfg              config.WrapperFilterConfig
	localImportFiles map[string]string
	newInstanceRoots map[string]string
}

func buildTSOwnerContext(filePath string, astInfo *types.ASTInfo, cfg *config.Config) tsOwnerContext {
	ctx := tsOwnerContext{
		filePath:         filePath,
		localImportFiles: make(map[string]string),
		newInstanceRoots: make(map[string]string),
	}
	if cfg != nil {
		ctx.cfg = cfg.WrapperFilter
	} else {
		ctx.cfg = config.DefaultConfig().WrapperFilter
	}
	if astInfo == nil {
		return ctx
	}

	for _, imp := range astInfo.Imports {
		if !isLocalImportPath(imp.Path) {
			continue
		}
		resolved := resolveLocalImportFile(filePath, imp.Path)
		if resolved == "" {
			continue
		}
		for _, symbol := range extractImportSymbols(imp) {
			ctx.localImportFiles[symbol] = resolved
		}
	}

	for _, c := range astInfo.Constants {
		if className := extractNewInstanceClass(c.Value); className != "" {
			ctx.newInstanceRoots[c.Name] = className
		}
	}
	for _, v := range astInfo.Variables {
		if className := extractNewInstanceClass(v.Value); className != "" {
			ctx.newInstanceRoots[v.Name] = className
		}
	}

	return ctx
}

func (ctx tsOwnerContext) classify(callee, fullReceiver, funcName string) (string, string, string, string) {
	ownerRoot := extractOwnerRoot(fullReceiver)
	if ctx.isForceInfraMethod(funcName) {
		return ownerRoot, "infrastructure", "config_force_infra_method", ctx.ownerFileForRoot(ownerRoot)
	}
	if matchesAnyPattern(callee, ctx.cfg.ForceInfraCallPatterns) {
		return ownerRoot, "infrastructure", "config_force_infra", ""
	}
	if matchesAnyPattern(callee, ctx.cfg.ForceBusinessCallPatterns) {
		return ownerRoot, "business", "config_force_business", ctx.ownerFileForRoot(ownerRoot)
	}
	if ownerRoot == "" {
		if isPromptSafetyInfraCall(callee, funcName) {
			return "", "infrastructure", "safety_no_receiver", ""
		}
		return "", "unknown", "no_owner_root", ""
	}
	if ctx.isKnownInfraRoot(ownerRoot) || hasObviousInfraSegment(fullReceiver) {
		return ownerRoot, "infrastructure", "known_infra_root", ""
	}
	ownerFile := ctx.ownerFileForRoot(ownerRoot)
	if ownerFile != "" {
		if ctx.looksLikeElementAction(fullReceiver, funcName) {
			return ownerRoot, "infrastructure", "element_like_property", ownerFile
		}
		return ownerRoot, "business", "local_import_owner", ownerFile
	}
	if className, ok := ctx.newInstanceRoots[ownerRoot]; ok {
		ownerFile = ctx.localImportFiles[className]
		if ctx.looksLikeElementAction(fullReceiver, funcName) {
			return ownerRoot, "infrastructure", "element_like_property", ownerFile
		}
		if ownerFile != "" {
			return ownerRoot, "business", "local_import_new_instance", ownerFile
		}
	}
	if matchesAnyPattern(ownerRoot, ctx.cfg.KnownBusinessNamePatterns) {
		if ctx.looksLikeElementAction(fullReceiver, funcName) {
			return ownerRoot, "infrastructure", "element_like_property", ownerFile
		}
		return ownerRoot, "business", "business_name_pattern", ownerFile
	}
	if isPromptSafetyInfraCall(callee, funcName) || ctx.looksLikeElementAction(fullReceiver, funcName) {
		return ownerRoot, "infrastructure", "safety_infra", ownerFile
	}
	return ownerRoot, "unknown", "fallback_unknown", ownerFile
}

func (ctx tsOwnerContext) ownerFileForRoot(root string) string {
	if root == "" {
		return ""
	}
	if className, ok := ctx.newInstanceRoots[root]; ok {
		if ownerFile := ctx.localImportFiles[className]; ownerFile != "" {
			return ownerFile
		}
	}
	if ownerFile := ctx.localImportFiles[root]; ownerFile != "" {
		return ownerFile
	}
	if ownerFile := inferPageObjectFileForRoot(ctx.filePath, root); ownerFile != "" {
		return ownerFile
	}
	return ""
}

func inferPageObjectFileForRoot(filePath, root string) string {
	root = strings.TrimSpace(root)
	if filePath == "" || root == "" {
		return ""
	}

	baseName := upperFirst(root)
	if !strings.HasSuffix(baseName, "Page") {
		return ""
	}

	for _, searchRoot := range findTSPageObjectSearchRoots(filePath) {
		candidates := scanTSFilesByBaseName([]string{searchRoot}, baseName)
		if len(candidates) > 0 {
			return candidates[0]
		}
	}
	return ""
}

func findTSPageObjectSearchRoots(caseFilePath string) []string {
	if caseFilePath == "" {
		return nil
	}
	caseDir := filepath.Dir(caseFilePath)

	var roots []string
	seen := make(map[string]struct{})
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			return
		}
		seen[p] = struct{}{}
		roots = append(roots, p)
	}

	cur := caseDir
	for i := 0; i < 8; i++ {
		add(filepath.Join(cur, "pages"))
		add(filepath.Join(cur, "pageObjects"))
		add(filepath.Join(cur, "pageobjects"))
		add(filepath.Join(cur, "new_pages"))
		if filepath.Base(cur) == "e2e" {
			add(filepath.Join(cur, "pages"))
			add(filepath.Join(cur, "pages", "new_pages"))
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	sort.Strings(roots)
	return roots
}

func scanTSFilesByBaseName(roots []string, baseName string) []string {
	baseName = strings.TrimSpace(baseName)
	if baseName == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var hits []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				switch d.Name() {
				case ".git", "node_modules", "dist":
					return filepath.SkipDir
				}
				return nil
			}
			if !isTSScriptFile(d.Name()) {
				return nil
			}
			if strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())) != baseName {
				return nil
			}
			clean := filepath.Clean(path)
			if _, ok := seen[clean]; ok {
				return nil
			}
			seen[clean] = struct{}{}
			hits = append(hits, clean)
			return nil
		})
	}
	sort.Strings(hits)
	return hits
}

func isTSScriptFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts":
		return true
	default:
		return false
	}
}

func upperFirst(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (ctx tsOwnerContext) isKnownInfraRoot(root string) bool {
	root = strings.ToLower(strings.TrimSpace(root))
	for _, item := range ctx.cfg.KnownInfraRoots {
		if root == strings.ToLower(strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func (ctx tsOwnerContext) isForceInfraMethod(funcName string) bool {
	return containsNormalized(ctx.cfg.ForceInfraMethods, funcName)
}

func (ctx tsOwnerContext) looksLikeElementAction(fullReceiver, funcName string) bool {
	if !containsNormalized(ctx.cfg.InfraTerminalMethods, funcName) {
		return false
	}
	for _, seg := range ownerPropertySegments(fullReceiver) {
		if matchesAnyPattern(seg, ctx.cfg.ElementLikePropertyPatterns) {
			return true
		}
	}
	return false
}

func extractOwnerRoot(fullReceiver string) string {
	parts := strings.Split(fullReceiver, ".")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "this" {
			continue
		}
		return part
	}
	return ""
}

func ownerPropertySegments(fullReceiver string) []string {
	parts := strings.Split(fullReceiver, ".")
	rootSeen := false
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "this" {
			continue
		}
		if !rootSeen {
			rootSeen = true
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

func containsNormalized(items []string, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, item := range items {
		if value == strings.ToLower(strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func hasObviousInfraSegment(fullReceiver string) bool {
	for _, seg := range strings.Split(fullReceiver, ".") {
		seg = strings.ToLower(strings.TrimSpace(seg))
		if _, ok := obviousInfraSegments[seg]; ok {
			return true
		}
	}
	return false
}

func isPromptSafetyInfraCall(callee, funcName string) bool {
	if _, ok := promptSafetyInfraMethods[strings.TrimSpace(funcName)]; ok {
		return true
	}
	for _, prefix := range []string{"console.", "JSON.", "Math.", "Object.", "Array.", "Promise.", "expect.", "assert."} {
		if strings.HasPrefix(callee, prefix) {
			return true
		}
	}
	return false
}

func extractNewInstanceClass(value string) string {
	if m := tsNewInstanceRe.FindStringSubmatch(value); len(m) == 2 {
		return m[1]
	}
	return ""
}

func extractImportSymbols(imp types.ImportInfo) []string {
	seen := make(map[string]struct{})
	var symbols []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		name = strings.TrimPrefix(name, "type ")
		if name == "" {
			return
		}
		if idx := strings.Index(name, " as "); idx >= 0 {
			name = strings.TrimSpace(name[idx+4:])
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		symbols = append(symbols, name)
	}

	for _, candidate := range []string{imp.Alias, imp.Name} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if start := strings.Index(candidate, "{"); start >= 0 {
			end := strings.Index(candidate, "}")
			if end > start {
				for _, part := range strings.Split(candidate[start+1:end], ",") {
					add(part)
				}
				candidate = strings.TrimSpace(candidate[:start])
			}
		}
		for _, part := range strings.Split(candidate, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "* as ") {
				part = strings.TrimSpace(strings.TrimPrefix(part, "* as "))
			}
			add(part)
		}
	}

	return symbols
}

func matchesAnyPattern(value string, patterns []string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		matched, err := regexp.MatchString(pattern, value)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func isLocalImportPath(importPath string) bool {
	return strings.HasPrefix(importPath, ".")
}

func resolveLocalImportFile(baseFile, importPath string) string {
	baseDir := filepath.Dir(baseFile)
	resolvedBase := filepath.Clean(filepath.Join(baseDir, importPath))
	for _, candidate := range buildImportCandidates(resolvedBase) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func buildImportCandidates(base string) []string {
	if ext := filepath.Ext(base); ext != "" {
		return []string{base}
	}
	var candidates []string
	exts := []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts"}
	for _, ext := range exts {
		candidates = append(candidates, base+ext)
	}
	for _, ext := range exts {
		candidates = append(candidates, filepath.Join(base, "index"+ext))
	}
	return candidates
}

func extractTSCallArguments(line string, openParenIdx int) ([]string, int) {
	if openParenIdx < 0 || openParenIdx >= len(line) || line[openParenIdx] != '(' {
		return nil, openParenIdx
	}

	depth := 0
	start := openParenIdx + 1
	var current strings.Builder
	var args []string

	for idx := openParenIdx; idx < len(line); idx++ {
		ch := line[idx]
		switch ch {
		case '(':
			depth++
			if depth > 1 {
				current.WriteByte(ch)
			}
		case ')':
			depth--
			if depth == 0 {
				arg := strings.TrimSpace(current.String())
				if arg != "" {
					args = append(args, arg)
				}
				return args, idx + 1
			}
			current.WriteByte(ch)
		case ',':
			if depth == 1 {
				arg := strings.TrimSpace(current.String())
				if arg != "" {
					args = append(args, arg)
				}
				current.Reset()
				continue
			}
			current.WriteByte(ch)
		default:
			if idx >= start {
				current.WriteByte(ch)
			}
		}
	}

	arg := strings.TrimSpace(current.String())
	if arg != "" {
		args = append(args, arg)
	}
	return args, len(line)
}

func enclosingTSRangeName(lineNo int, ranges []tsRange) string {
	for _, rng := range ranges {
		if lineNo >= rng.startLine && lineNo <= rng.endLine {
			return rng.name
		}
	}
	return ""
}
