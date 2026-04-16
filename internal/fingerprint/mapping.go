package fingerprint

import "github.com/sandaogouchen/nep2midsence/internal/types"

// NepToMidsceneMapping contains all known nep API to midscene API mappings
var NepToMidsceneMapping = map[string]*types.MigrationRule{
	// Navigation
	"nep.Navigate": {
		NepAPI: "nep.Navigate", MidsceneEquivalent: "page.goto(url)",
		NeedsIntentRewrite: false, Note: "URL 直接映射，无需转换",
	},
	// Element location
	"nep.FindElement": {
		NepAPI: "nep.FindElement", MidsceneEquivalent: "ai.locator('元素描述')",
		NeedsIntentRewrite: true, Note: "CSS/XPath 选择器需转为自然语言描述",
		Examples: []types.Example{
			{Before: `nep.FindElement("#login-btn")`, After: `ai.locator("登录按钮")`},
			{Before: `nep.FindElement(".nav-menu > li:nth-child(2)")`, After: `ai.locator("导航菜单的第二项")`},
		},
	},
	// Actions
	"nep.Click":       {NepAPI: "nep.Click", MidsceneEquivalent: "ai.action('点击...')", NeedsIntentRewrite: true},
	"nep.SendKeys":    {NepAPI: "nep.SendKeys", MidsceneEquivalent: "ai.action('在...输入...')", NeedsIntentRewrite: true},
	"nep.Clear":       {NepAPI: "nep.Clear", MidsceneEquivalent: "ai.action('清空...')", NeedsIntentRewrite: true},
	"nep.Hover":       {NepAPI: "nep.Hover", MidsceneEquivalent: "ai.action('悬停在...')", NeedsIntentRewrite: true},
	"nep.DoubleClick": {NepAPI: "nep.DoubleClick", MidsceneEquivalent: "ai.action('双击...')", NeedsIntentRewrite: true},
	// Waits
	"nep.WaitForElement": {NepAPI: "nep.WaitForElement", MidsceneEquivalent: "ai.assert('...')", NeedsIntentRewrite: true, Note: "显式等待转为 AI 断言"},
	"nep.WaitForVisible": {NepAPI: "nep.WaitForVisible", MidsceneEquivalent: "ai.assert('...可见')", NeedsIntentRewrite: true},
	// Info retrieval
	"nep.GetText":      {NepAPI: "nep.GetText", MidsceneEquivalent: "ai.query('获取...的文本')", NeedsIntentRewrite: true},
	"nep.GetAttribute": {NepAPI: "nep.GetAttribute", MidsceneEquivalent: "ai.query('获取...的属性')", NeedsIntentRewrite: true},
	// Screenshots
	"nep.Screenshot": {NepAPI: "nep.Screenshot", MidsceneEquivalent: "page.screenshot()", NeedsIntentRewrite: false},
	// Browser operations
	"nep.SwitchToFrame":  {NepAPI: "nep.SwitchToFrame", MidsceneEquivalent: "// midscene iframe 处理", NeedsIntentRewrite: true, Note: "高复杂度，需要查阅 midscene iframe 文档"},
	"nep.SwitchToWindow": {NepAPI: "nep.SwitchToWindow", MidsceneEquivalent: "// midscene 多标签页处理", NeedsIntentRewrite: true, Note: "高复杂度，需要查阅 midscene 多标签页文档"},
}
