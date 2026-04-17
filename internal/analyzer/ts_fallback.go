package analyzer

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

var (
	tsImportFromRe  = regexp.MustCompile(`^\s*import\s+(.+?)\s+from\s+['"]([^'"]+)['"]`)
	tsImportBareRe  = regexp.MustCompile(`^\s*import\s+['"]([^'"]+)['"]`)
	tsVarDeclRe     = regexp.MustCompile(`^\s*(?:export\s+)?(const|let|var)\s+([A-Za-z_$][\w$]*)[^=]*=\s*(.+?)\s*;?\s*$`)
	tsFnDeclRe      = regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(`)
	tsArrowFnDeclRe = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?\([^)]*\)\s*=>`)
	tsTestCallRe     = regexp.MustCompile(`\b(it|test)\s*\(\s*['"` + "`" + `]([^'"` + "`" + `]+)['"` + "`" + `]\s*,`)
	tsTestCallBareRe = regexp.MustCompile(`^\s*(?:commonIt|it|test)\s*\(\s*$`)
	tsTestNameRe     = regexp.MustCompile(`^\s*['"` + "`" + `]([^'"` + "`" + `]+)['"` + "`" + `]\s*,`)
	tsHookCallRe     = regexp.MustCompile(`\b(beforeEach|afterEach|beforeAll|afterAll)\s*\(`)
	// Capture multi-level property access chains like:
	//   adGroupPage.optimizationAndBiddingModule1MNBA.vv_setStandardBtn(
	// Group 1: optional receiver chain with trailing dot ("adGroupPage.optimization...")
	// Group 2: terminal function/method name ("vv_setStandardBtn")
	tsCallRe        = regexp.MustCompile(`((?:[A-Za-z_$][\w$]*\.)+)?([A-Za-z_$][\w$]*)\s*\(`)
	// component prompt extraction
	tsClassDeclRe       = regexp.MustCompile(`\bclass\s+([A-Za-z_$][\w$]*)`)
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

var tsNepAPIs = map[string]struct{}{
	"navigate": {}, "goto": {}, "navigateTo": {},
	"click": {}, "dblclick": {}, "clickLogin": {}, "clickCreateCampaign": {},
	"sendKeys": {}, "type": {}, "fill": {},
	"fillUsername": {}, "fillPassword": {}, "setBudget": {}, "selectObjective": {},
	"findElement": {}, "querySelector": {},
	"findElements": {}, "querySelectorAll": {},
	"waitForElement": {}, "waitForSelector": {},
	"waitForNavigation": {}, "waitForTimeout": {},
	"assertVisible": {}, "assertText": {}, "assertExists": {}, "expect": {},
	"getText": {}, "getInnerText": {}, "getAttribute": {},
	"create": {}, "close": {}, "login": {},
}

func extractTypeScriptFallback(filePath string) (*types.ASTInfo, []types.CallStep, string, error) {
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

	allCalls := collectTSCalls(filePath, lines, testRanges, hookRanges)
	return astInfo, allCalls, language, nil
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

func collectTSCalls(filePath string, lines []string, ranges ...[]tsRange) []types.CallStep {
	var allRanges []tsRange
	for _, group := range ranges {
		allRanges = append(allRanges, group...)
	}

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
			steps = append(steps, types.CallStep{
				Callee:    callee,
				Receiver:  receiver,
				FullReceiver: fullReceiver,
				FuncName:  funcName,
				Args:      args,
				FilePath:  filePath,
				Line:      lineNo,
				IsNepAPI:  isNepAPI,
				IsNep:     isNepAPI,
				IsAwait:   strings.Contains(line[:match[0]], "await "),
				IsChained: receiver != "",
				IsWrapperCall: strings.Contains(fullReceiver, "."),
				InFunc:    enclosingTSRangeName(lineNo, allRanges),
				Comment:   strings.TrimSpace(line[endIdx:]),
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
