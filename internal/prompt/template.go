package prompt

const migrationTemplate = `## 迁移任务

**严格按照以下指令执行，不要做额外的事情。**

---

### 1. 迁移参考文档

{{if .MigrationDoc}}
{{.MigrationDoc}}
{{else}}
（无额外迁移文档）
{{end}}

{{if .IsCrossRepo}}
---

### 跨仓库迁移说明

本次为跨仓库迁移：
- 源仓库：` + "`" + `{{.SourceRepoRoot}}` + "`" + `
- 目标仓库：` + "`" + `{{.TargetRepoRoot}}` + "`" + `

请 Read 源文件后将迁移代码写入目标路径。输出文件中不得残留任何 NEP import 或调用。
{{end}}

---

### 2. 本 Case 涉及的 API 映射表

| nep API | midscene 等价写法 | 说明 |
|---|---|---|
{{range .APIMappings -}}
| ` + "`" + `{{.NepAPI}}` + "`" + ` | ` + "`" + `{{.MidsceneAPI}}` + "`" + ` | {{.Note}} |
{{end}}

---

### 3. 操作步骤序列（展平后的调用链）

以下是原始 case 展开后的完整操作序列：

{{range .OperationSteps -}}
**Step {{add .Index 1}}** [{{.StepType}}]: {{.Intent}}
  原始调用: ` + "`" + `{{.NepCall}}` + "`" + `
  目标写法: ` + "`" + `{{.MidsceneCall}}` + "`" + `

{{end}}

{{if .HelperPlan}}
---

### 3.6 最小 helper 迁移范围

- 来源 receiver：` + "`" + `{{.HelperPlan.Receiver}}` + "`" + `
- 仅迁移以下方法：
{{range .HelperPlan.Methods -}}
  - ` + "`" + `{{.}}` + "`" + `
{{end}}
{{if .HelperPlan.PageObjectFile -}}
- 所属 page object：` + "`" + `{{.HelperPlan.PageObjectFile}}` + "`" + `
{{end}}

{{end}}

{{if .UnresolvedHelpers}}
---

### 3.7 未解析 helper 依赖

以下 helper 依赖未成功定位或无法最小迁移，已按 receiver 可达性分类处理：

| receiver | method | reason | 可达性 | 策略 |
|---|---|---|---|---|
{{range .UnresolvedHelpers -}}
| ` + "`" + `{{.Receiver}}` + "`" + ` | ` + "`" + `{{.Method}}` + "`" + ` | {{.Reason}} | {{if .ReceiverReachable}}可达{{else}}不可达{{end}} | {{if .ReceiverReachable}}新建 Midscene 版本{{else}}保留原调用+TODO{{end}} |
{{end}}

{{end}}

{{if .DefaultPrompts}}
---

### 3.5 组件 DEFAULT_PROMPT 映射

以下是封装组件的 ` + "`" + `DEFAULT_PROMPT` + "`" + ` 描述，迁移时应优先使用这些描述生成 midscene agent 的自然语言参数：

| 组件类名 | DEFAULT_PROMPT 描述 | 来源文件 |
|---|---|---|
{{range .DefaultPrompts -}}
| ` + "`" + `{{.ClassName}}` + "`" + ` | {{.PromptValue}} | ` + "`" + `{{.FilePath}}` + "`" + ` |
{{end}}

{{end}}

---

### 4. 检测到的代码模式

{{if .Patterns -}}
{{range .Patterns -}}
- **{{.Type}}**: {{.Strategy}}
{{end}}
{{else -}}
（未检测到特殊模式）
{{end}}

---

### 5. 关键数据依赖

{{if .DataFlows -}}
{{range .DataFlows -}}
- 变量 ` + "`" + `{{.Variable}}` + "`" + `: {{.Kind}}，值为 ` + "`" + `{{.Value}}` + "`" + `，定义在 {{.DefinedAt}}
{{end}}
{{else -}}
（无特殊数据依赖）
{{end}}

---

### 6. 原始代码

本次迁移 **不要在 prompt 内联源码**。请通过 ` + "`" + `Read` + "`" + ` / ` + "`" + `Grep` + "`" + ` 工具读取以下文件内容后再开始修改：

- 源文件（需要迁移的 case/helper）：` + "`" + `{{.SourceFile}}` + "`" + `
- 目标输出文件（将迁移后的代码写入此路径）：` + "`" + `{{.TargetFile}}` + "`" + `

{{if .ReferenceDocs}}
#### 6.1 参考文档（先读）

{{range .ReferenceDocs -}}
- ` + "`" + `{{.}}` + "`" + `
{{end}}
{{end}}

{{if .RelatedFiles}}
#### 6.2 相关代码文件（按需 Read/Grep，用于封装函数/PO/helper 上下文）

{{range .RelatedFiles -}}
- ` + "`" + `{{.}}` + "`" + `
{{end}}
{{end}}

{{if .LocalImportDeps}}
#### 6.3 源仓库本地依赖 import（跨仓库时不得直接原样保留）

| import path | 当前文件实际引用 | 源文件 |
|---|---|---|
{{range .LocalImportDeps -}}
| ` + "`" + `{{.ImportPath}}` + "`" + ` | ` + "`" + `{{.ImportSpec}}` + "`" + ` | ` + "`" + `{{.SourceFile}}` + "`" + ` |
{{end}}

请优先 Read 上述源文件；若目标仓库没有同名模块，不要继续保留这些 import，必须改为目标仓库内可解析的实现（最小化内联或改写为本地依赖）。

**注意**：表中 SourceFile 标记为 "(unresolved alias ...)" 的行表示该 alias 在目标仓库无法解析，必须内联或改写。
{{end}}

{{if .SharedSymbolDeps}}
#### 6.4 共享符号依赖（禁止在 case 内重定义）

以下符号已经被识别为共享依赖。迁移时必须保留为 import，不得在 case 内重新声明同名 ` + "`" + `const` + "`" + ` / ` + "`" + `enum` + "`" + ` / ` + "`" + `function` + "`" + ` / noop stub：

| symbol | source import | import spec | concrete export file | target dependency file | kind |
|---|---|---|---|---|---|
{{range .SharedSymbolDeps -}}
| ` + "`" + `{{.ImportedName}}` + "`" + ` | ` + "`" + `{{.ImportPath}}` + "`" + ` | ` + "`" + `{{.ImportSpec}}` + "`" + ` | ` + "`" + `{{.ExportFile}}` + "`" + ` | ` + "`" + `{{.TargetFile}}` + "`" + ` | ` + "`" + `{{.DependencyKind}}` + "`" + ` |
{{end}}

处理要求：

- 禁止在 case 内重定义上述共享符号
- 必须从迁移后的目标依赖文件 import
- 若源仓库通过 barrel re-export 暴露该符号，迁移后应指向 concrete migrated dependency，而不是继续依赖不可解析的源仓库 alias
{{end}}

{{if .ExampleBefore}}
---

### 7. 参考示例

**迁移前：**
` + "```" + `{{.CodeFenceLang}}
{{.ExampleBefore}}
` + "```" + `

**迁移后：**
` + "```" + `{{.CodeFenceLang}}
{{.ExampleAfter}}
` + "```" + `
{{end}}

{{if .HasCommonItWrapper}}
---

### 7.5 commonIt Wrapper 约束

本 case 使用了 commonIt wrapper 封装，以下参数由 wrapper 自动注入，**不是真实 import 依赖**：

- **Wrapper 注入的参数**：page, midscene{{range .WrapperInjectedPageObjects}}, ` + "`" + `{{.}}` + "`" + `{{end}}
{{if .WrapperUrl}}- **Wrapper 自动导航 URL**：` + "`" + `{{.WrapperUrl}}` + "`" + ` — 迁移后需要在 test body 中显式调用 ` + "`" + `agent.aiAction("navigate to " + url)` + "`" + ` 或使用 ` + "`" + `page.goto(url)` + "`" + `{{end}}

**迁移规则**：
1. 将 commonIt 替换为标准 ` + "`" + `it` + "`" + ` / ` + "`" + `test` + "`" + `，回调签名改为 ` + "`" + `CaseFunctionParams<'test'>` + "`" + `，仅接收 ` + "`" + `{ page, midscene }` + "`" + `
2. Wrapper 注入的 Page Object 实例（如 {{range .WrapperInjectedPageObjects}}` + "`" + `{{.}}` + "`" + ` {{end}}）需在 test body 内自行 new 或从 import 获取
3. Wrapper 自动执行的 URL 导航需在 test body 开头显式补上
4. 删除对 commonIt wrapper 的 import
{{end}}

---

### 8. 输出要求

{{range $i, $c := .Constraints -}}
{{add $i 1}}. {{$c}}
{{end}}
`
