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

以下 helper 依赖未成功定位或无法最小迁移。请在 case 中保留原调用，并在对应调用前添加统一 TODO 注释。

| receiver | method | reason |
|---|---|---|
{{range .UnresolvedHelpers -}}
| ` + "`" + `{{.Receiver}}` + "`" + ` | ` + "`" + `{{.Method}}` + "`" + ` | {{.Reason}} |
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

---

### 8. 输出要求

{{range $i, $c := .Constraints -}}
{{add $i 1}}. {{$c}}
{{end}}
`
