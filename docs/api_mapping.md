# API 映射文档

## 完整映射表

| nep API | midscene 等价写法 | 需要意图改写 | 说明 |
|---|---|---|---|
| `nep.Navigate` | `page.goto(url)` | 否 | URL 直接映射 |
| `nep.FindElement` | `ai.locator('描述')` | 是 | 选择器需转为自然语言 |
| `nep.Click` | `ai.action('点击...')` | 是 | |
| `nep.SendKeys` | `ai.action('输入...')` | 是 | |
| `nep.Clear` | `ai.action('清空...')` | 是 | |
| `nep.Hover` | `ai.action('悬停...')` | 是 | |
| `nep.DoubleClick` | `ai.action('双击...')` | 是 | |
| `nep.WaitForElement` | `ai.assert('...')` | 是 | 显式等待转断言 |
| `nep.WaitForVisible` | `ai.assert('...可见')` | 是 | |
| `nep.GetText` | `ai.query('获取文本')` | 是 | |
| `nep.GetAttribute` | `ai.query('获取属性')` | 是 | |
| `nep.Screenshot` | `page.screenshot()` | 否 | 直接映射 |
| `nep.SwitchToFrame` | _需查阅文档_ | 是 | 高复杂度 |
| `nep.SwitchToWindow` | _需查阅文档_ | 是 | 高复杂度 |
