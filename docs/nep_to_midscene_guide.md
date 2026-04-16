# nep → midscene 迁移指南

## 核心差异

| 维度 | nep | midscene |
|---|---|---|
| 元素定位 | CSS/XPath 选择器 | 自然语言描述 |
| 操作方式 | 显式 API 调用 | AI 驱动的操作指令 |
| 等待策略 | 显式等待 | AI 断言 |
| 数据提取 | DOM 属性读取 | AI 查询 |

## API 映射速查

### 导航
- `nep.Navigate(url)` → `page.goto(url)`

### 元素定位与操作
- `nep.FindElement(selector).Click()` → `ai.action('点击 <元素描述>')`
- `nep.FindElement(selector).SendKeys(text)` → `ai.action('在 <元素描述> 中输入 <text>')`
- `nep.FindElement(selector).Clear()` → `ai.action('清空 <元素描述>')`
- `nep.FindElement(selector).Hover()` → `ai.action('悬停在 <元素描述> 上')`

### 等待
- `nep.WaitForElement(selector)` → `ai.assert('<元素描述> 已出现')`
- `nep.WaitForVisible(selector)` → `ai.assert('<元素描述> 可见')`

### 数据提取
- `nep.GetText(selector)` → `ai.query('获取 <元素描述> 的文本')`
- `nep.GetAttribute(selector, attr)` → `ai.query('获取 <元素描述> 的 <attr> 属性')`

## 迁移注意事项

1. **选择器转自然语言**：将 CSS 选择器转换为人类可理解的元素描述
2. **保留测试逻辑**：只替换 nep API 调用，保留断言和测试数据
3. **Page Object 模式**：保留 PO 结构，只替换内部实现
4. **数据驱动测试**：保留数据表和循环结构，替换操作方式
