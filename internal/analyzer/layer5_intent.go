package analyzer

import (
	"strings"
	"unicode"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// IntentAnalyzer infers business intent for each operation step in a test
// function's call chain. It produces human-readable (Chinese) descriptions
// that downstream migration tools can embed as comments or midscene ai*
// intent parameters.
type IntentAnalyzer struct {
	// verbMap maps nep API verb prefixes to Chinese action descriptions.
	verbMap map[string]string
	// elementMap maps common selector fragments to Chinese element names.
	elementMap map[string]string
}

// NewIntentAnalyzer returns an IntentAnalyzer pre-loaded with common term
// mappings.
func NewIntentAnalyzer() *IntentAnalyzer {
	return &IntentAnalyzer{
		verbMap: map[string]string{
			"Click":          "点击",
			"DoubleClick":    "双击",
			"RightClick":     "右键点击",
			"SendKeys":       "输入",
			"Type":           "输入",
			"Clear":          "清空",
			"Submit":         "提交",
			"Navigate":       "导航到",
			"Open":           "打开",
			"Get":            "获取",
			"GetText":        "获取文本",
			"GetAttribute":   "获取属性",
			"FindElement":    "查找元素",
			"FindElements":   "查找多个元素",
			"WaitFor":        "等待",
			"WaitForElement": "等待元素出现",
			"WaitForVisible": "等待元素可见",
			"Assert":         "断言",
			"Verify":         "验证",
			"Check":          "勾选",
			"Uncheck":        "取消勾选",
			"Select":         "选择",
			"Hover":          "悬停在",
			"DragAndDrop":    "拖拽",
			"Scroll":         "滚动到",
			"ScrollTo":       "滚动到",
			"SwitchToFrame":  "切换到iframe",
			"SwitchToWindow": "切换到窗口",
			"Screenshot":     "截图",
			"Upload":         "上传文件",
			"Download":       "下载文件",
			"Close":          "关闭",
			"Back":           "返回上一页",
			"Forward":        "前进到下一页",
			"Refresh":        "刷新页面",
		},
		elementMap: map[string]string{
			"login":    "登录",
			"logout":   "登出",
			"submit":   "提交",
			"search":   "搜索",
			"username": "用户名",
			"password": "密码",
			"email":    "邮箱",
			"phone":    "手机号",
			"btn":      "按钮",
			"button":   "按钮",
			"input":    "输入框",
			"text":     "文本",
			"link":     "链接",
			"tab":      "标签页",
			"menu":     "菜单",
			"nav":      "导航",
			"header":   "页头",
			"footer":   "页脚",
			"sidebar":  "侧边栏",
			"modal":    "弹窗",
			"dialog":   "对话框",
			"confirm":  "确认",
			"cancel":   "取消",
			"delete":   "删除",
			"edit":     "编辑",
			"save":     "保存",
			"add":      "添加",
			"create":   "创建",
			"upload":   "上传",
			"download": "下载",
			"close":    "关闭",
			"open":     "打开",
			"select":   "选择",
			"checkbox": "复选框",
			"radio":    "单选框",
			"dropdown": "下拉框",
			"table":    "表格",
			"row":      "行",
			"column":   "列",
			"form":     "表单",
			"title":    "标题",
			"name":     "名称",
			"desc":     "描述",
			"img":      "图片",
			"image":    "图片",
			"icon":     "图标",
			"avatar":   "头像",
		},
	}
}

// Analyze produces IntentInfo for each test function referenced in the call
// chains. For every step that is a nep API call, it infers business intent
// using the priority: inline comment > function name > selector semantics >
// context.
func (a *IntentAnalyzer) Analyze(chains []*types.CallChain, astInfo *types.ASTInfo) []*types.IntentInfo {
	// Build a quick-lookup map from function name to FuncInfo.
	funcMap := make(map[string]*types.FuncInfo)
	for i := range astInfo.Functions {
		funcMap[astInfo.Functions[i].Name] = &astInfo.Functions[i]
	}

	var results []*types.IntentInfo

	for _, chain := range chains {
		info := &types.IntentInfo{
			FuncName: chain.EntryFunc,
		}

		// Function-level intent.
		if fn, ok := funcMap[chain.EntryFunc]; ok {
			info.FuncDoc = fn.Doc
			info.FuncIntent = a.inferFuncIntent(fn)
		}

		// Step-level intents.
		for idx, step := range chain.Steps {
			if !step.IsNepAPI {
				continue
			}
			si := a.inferStepIntent(step, idx)
			info.StepIntents = append(info.StepIntents, si)
		}

		results = append(results, info)
	}

	return results
}

// inferFuncIntent derives a high-level intent for the entire test function.
func (a *IntentAnalyzer) inferFuncIntent(fn *types.FuncInfo) string {
	// Prefer existing doc comment.
	if fn.Doc != "" {
		return strings.TrimSpace(fn.Doc)
	}
	// Fall back to natural-language expansion of the function name.
	return camelToNaturalLanguage(fn.Name)
}

// inferStepIntent infers the business intent for a single nep API call step.
func (a *IntentAnalyzer) inferStepIntent(step types.CallStep, idx int) types.StepIntent {
	si := types.StepIntent{
		StepIndex:  idx,
		NepAPICall: step.Callee,
	}

	// Priority 1: inline comment.
	if step.Comment != "" {
		si.InlineComment = step.Comment
		si.InferredIntent = step.Comment
		si.IntentSource = "inline_comment"
		si.Confidence = 0.95
		return si
	}

	// Priority 2: function/method name semantics.
	verb, element := a.parseCallee(step.Callee)
	actionDesc := a.verbToAction(verb)

	// Priority 3: selector semantics from arguments.
	selectorDesc := ""
	if len(step.Args) > 0 {
		selectorDesc = a.selectorToElementName(step.Args[0])
	}

	// Compose intent.
	switch {
	case actionDesc != "" && selectorDesc != "":
		si.InferredIntent = actionDesc + selectorDesc
		si.IntentSource = "selector_semantics"
		si.Confidence = 0.80
	case actionDesc != "" && element != "":
		si.InferredIntent = actionDesc + element
		si.IntentSource = "method_name"
		si.Confidence = 0.70
	case actionDesc != "":
		si.InferredIntent = actionDesc + "目标元素"
		si.IntentSource = "verb_only"
		si.Confidence = 0.50
	default:
		si.InferredIntent = "执行 " + step.Callee
		si.IntentSource = "context"
		si.Confidence = 0.30
	}

	return si
}

// selectorToElementName converts a CSS selector to a natural-language Chinese
// description.
//
// Examples:
//
//	#login-btn   → "登录按钮"
//	.submit-button → "提交按钮"
//	#username    → "用户名输入框"
func (a *IntentAnalyzer) selectorToElementName(selector string) string {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return ""
	}

	// Strip leading # or . prefix to get the raw name.
	raw := selector
	if strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, ".") {
		raw = raw[1:]
	}
	// Also strip attribute-selector wrappers like [name="xxx"]
	if strings.HasPrefix(raw, "[") {
		raw = strings.Trim(raw, "[]")
		if eqIdx := strings.Index(raw, "="); eqIdx >= 0 {
			raw = strings.Trim(raw[eqIdx+1:], "\"' ")
		}
	}

	// Split by common delimiters.
	parts := splitIdentifier(raw)

	// Map each part to Chinese where possible; collect results.
	var translated []string
	for _, p := range parts {
		p = strings.ToLower(p)
		if ch, ok := a.elementMap[p]; ok {
			translated = append(translated, ch)
		}
	}

	if len(translated) == 0 {
		return raw
	}

	return strings.Join(translated, "")
}

// camelToNaturalLanguage splits a camelCase or snake_case identifier into
// space-separated lowercase words.
func camelToNaturalLanguage(name string) string {
	// Remove common test prefixes.
	name = strings.TrimPrefix(name, "Test")
	name = strings.TrimPrefix(name, "test")
	name = strings.TrimPrefix(name, "test_")

	parts := splitIdentifier(name)
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// parseCallee splits "receiver.Method" into a verb portion and an optional
// element portion. For example "page.ClickLoginButton" returns ("Click",
// "LoginButton").
func (a *IntentAnalyzer) parseCallee(callee string) (verb, element string) {
	// Take the part after the last dot.
	method := callee
	if idx := strings.LastIndex(callee, "."); idx >= 0 {
		method = callee[idx+1:]
	}

	// Try to match the longest known verb prefix.
	bestLen := 0
	for v := range a.verbMap {
		if strings.HasPrefix(method, v) && len(v) > bestLen {
			verb = v
			bestLen = len(v)
		}
	}

	if bestLen > 0 {
		element = method[bestLen:]
	} else {
		verb = method
	}

	return verb, element
}

// verbToAction returns the Chinese action description for a verb, or an empty
// string if unknown.
func (a *IntentAnalyzer) verbToAction(verb string) string {
	if ch, ok := a.verbMap[verb]; ok {
		return ch
	}
	return ""
}

// splitIdentifier splits a camelCase, PascalCase, or snake_case identifier
// into individual words.
func splitIdentifier(name string) []string {
	// First split on underscores and hyphens.
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// Then split camelCase boundaries.
	var parts []string
	var current strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if r == ' ' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			continue
		}
		if unicode.IsUpper(r) && i > 0 && runes[i-1] != ' ' {
			// Boundary: lowercase->uppercase or end of uppercase run.
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}
