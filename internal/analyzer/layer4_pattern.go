package analyzer

import (
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// PatternDetector detects code patterns in test files and assigns migration
// strategies for each recognised pattern.
type PatternDetector struct{}

// NewPatternDetector returns a new PatternDetector.
func NewPatternDetector() *PatternDetector {
	return &PatternDetector{}
}

// Detect inspects the AST information and call chains to identify which of the
// seven known patterns are present, assigns a migration strategy for each, and
// computes an overall complexity rating.
func (d *PatternDetector) Detect(astInfo *types.ASTInfo, chains []*types.CallChain) *types.PatternResult {
	result := &types.PatternResult{
		Detected:   []types.PatternType{},
		Strategies: make(map[types.PatternType]string),
	}

	// Run every detector. Order does not matter.
	d.detectPageObject(astInfo, chains, result)
	d.detectChainCall(chains, result)
	d.detectDataDriven(astInfo, result)
	d.detectSetupTeardown(astInfo, result)
	d.detectExplicitWait(chains, result)
	d.detectIframe(chains, result)
	d.detectMultiTab(chains, result)

	// Determine overall complexity.
	n := len(result.Detected)
	switch {
	case n <= 1:
		result.Complexity = "simple"
	case n <= 3:
		result.Complexity = "medium"
	default:
		result.Complexity = "complex"
	}

	return result
}

// ---------------------------------------------------------------------------
// Individual pattern detectors
// ---------------------------------------------------------------------------

// detectPageObject checks for structs whose methods contain nep API calls.
func (d *PatternDetector) detectPageObject(astInfo *types.ASTInfo, chains []*types.CallChain, result *types.PatternResult) {
	if len(astInfo.Structs) == 0 {
		return
	}

	// Build a set of nep API callee names for quick lookup.
	nepCalls := nepCalleeSet(chains)

	for _, s := range astInfo.Structs {
		for _, method := range s.Methods {
			// Find the FuncInfo for this method and check if it touches nep APIs.
			for _, fn := range astInfo.Functions {
				if fn.Name == method && fn.Receiver != "" {
					if bodyContainsAny(fn.Body, nepCalls) {
						addPattern(result, types.PatternPageObject,
							"将 Page Object 结构体转换为 midscene Page 对象，方法内的 nep 调用替换为 ai* 意图调用")
						return
					}
				}
			}
		}
	}
}

// detectChainCall looks for sequential method chains on the same receiver
// (e.g. page.Click(...).SendKeys(...).Submit(...)).
func (d *PatternDetector) detectChainCall(chains []*types.CallChain, result *types.PatternResult) {
	for _, chain := range chains {
		if len(chain.NepAPICalls) < 2 {
			continue
		}
		prev := ""
		consecutive := 0
		for _, step := range chain.NepAPICalls {
			receiver := receiverFromCallee(step.Callee)
			if receiver != "" && receiver == prev {
				consecutive++
			} else {
				consecutive = 1
			}
			prev = receiver
			if consecutive >= 2 {
				addPattern(result, types.PatternChainCall,
					"将链式调用拆解为逐步的 midscene ai* 意图调用，保持执行顺序")
				return
			}
		}
	}
}

// detectDataDriven looks for for-range loops in function bodies that contain
// nep API calls – a hallmark of data-driven / table-driven tests.
func (d *PatternDetector) detectDataDriven(astInfo *types.ASTInfo, result *types.PatternResult) {
	nepKeywords := []string{"Click", "SendKeys", "FindElement", "WaitFor", "Navigate", "GetText"}
	for _, fn := range astInfo.Functions {
		body := fn.Body
		if body == "" {
			continue
		}
		hasRange := strings.Contains(body, "for ") && strings.Contains(body, "range ")
		if !hasRange {
			continue
		}
		for _, kw := range nepKeywords {
			if strings.Contains(body, kw) {
				addPattern(result, types.PatternDataDriven,
					"保留数据驱动结构（for-range），将循环体内的 nep 调用替换为 midscene ai* 意图调用")
				return
			}
		}
	}
}

// detectSetupTeardown checks for init(), TestMain, BeforeSuite, or AfterSuite.
func (d *PatternDetector) detectSetupTeardown(astInfo *types.ASTInfo, result *types.PatternResult) {
	setupNames := map[string]bool{
		"init":        true,
		"TestMain":    true,
		"BeforeSuite": true,
		"AfterSuite":  true,
	}

	for _, fn := range astInfo.Functions {
		if setupNames[fn.Name] {
			addPattern(result, types.PatternSetupTeardown,
				"将 setup/teardown 逻辑迁移至 midscene 的 BeforeAll/AfterAll 或 TestMain 钩子")
			return
		}
	}

	for _, init := range astInfo.InitBlocks {
		if init.Kind == "init" || init.Kind == "TestMain" {
			addPattern(result, types.PatternSetupTeardown,
				"将 setup/teardown 逻辑迁移至 midscene 的 BeforeAll/AfterAll 或 TestMain 钩子")
			return
		}
	}
}

// detectExplicitWait detects calls to WaitForElement or time.Sleep.
func (d *PatternDetector) detectExplicitWait(chains []*types.CallChain, result *types.PatternResult) {
	waitKeywords := []string{"WaitForElement", "WaitForVisible", "WaitFor", "time.Sleep"}
	for _, chain := range chains {
		for _, step := range chain.Steps {
			for _, kw := range waitKeywords {
				if strings.Contains(step.Callee, kw) {
					addPattern(result, types.PatternExplicitWait,
						"移除显式等待，改用 midscene 的内建智能等待；time.Sleep 替换为 aiWaitFor 意图断言")
					return
				}
			}
		}
	}
}

// detectIframe detects calls to SwitchToFrame.
func (d *PatternDetector) detectIframe(chains []*types.CallChain, result *types.PatternResult) {
	for _, chain := range chains {
		for _, step := range chain.Steps {
			if strings.Contains(step.Callee, "SwitchToFrame") || strings.Contains(step.Callee, "SwitchFrame") {
				addPattern(result, types.PatternIframe,
					"移除 iframe 切换调用，midscene 视觉模式可直接定位跨 iframe 元素")
				return
			}
		}
	}
}

// detectMultiTab detects calls to SwitchToWindow or NewTab.
func (d *PatternDetector) detectMultiTab(chains []*types.CallChain, result *types.PatternResult) {
	tabKeywords := []string{"SwitchToWindow", "SwitchWindow", "NewTab", "OpenTab"}
	for _, chain := range chains {
		for _, step := range chain.Steps {
			for _, kw := range tabKeywords {
				if strings.Contains(step.Callee, kw) {
					addPattern(result, types.PatternMultiTab,
						"使用 midscene 的多标签页管理 API 替换原有的窗口/标签页切换逻辑")
					return
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// addPattern appends a pattern only if it hasn't been added yet.
func addPattern(result *types.PatternResult, pt types.PatternType, strategy string) {
	for _, existing := range result.Detected {
		if existing == pt {
			return
		}
	}
	result.Detected = append(result.Detected, pt)
	result.Strategies[pt] = strategy
}

// nepCalleeSet builds a set of callee names from nep API calls across all chains.
func nepCalleeSet(chains []*types.CallChain) map[string]bool {
	m := make(map[string]bool)
	for _, chain := range chains {
		for _, step := range chain.NepAPICalls {
			m[step.Callee] = true
			// Also add the short form (after the dot).
			if idx := strings.LastIndex(step.Callee, "."); idx >= 0 {
				m[step.Callee[idx+1:]] = true
			}
		}
	}
	return m
}

// bodyContainsAny returns true if the function body contains any of the keys.
func bodyContainsAny(body string, keys map[string]bool) bool {
	for k := range keys {
		if strings.Contains(body, k) {
			return true
		}
	}
	return false
}

// receiverFromCallee extracts the receiver portion from "receiver.Method".
func receiverFromCallee(callee string) string {
	idx := strings.LastIndex(callee, ".")
	if idx <= 0 {
		return ""
	}
	return callee[:idx]
}
