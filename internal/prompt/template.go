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
**Step {{add .Index 1}}**: {{.Intent}}
  原始调用: ` + "`" + `{{.NepCall}}` + "`" + `
  目标写法: ` + "`" + `{{.MidsceneCall}}` + "`" + `

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

` + "```go" + `
{{.SourceCode}}
` + "```" + `

{{if .HelperCode}}
### 6.1 依赖的 Helper/Page Object 代码

` + "```go" + `
{{.HelperCode}}
` + "```" + `
{{end}}

{{if .ExampleBefore}}
---

### 7. 参考示例

**迁移前：**
` + "```go" + `
{{.ExampleBefore}}
` + "```" + `

**迁移后：**
` + "```go" + `
{{.ExampleAfter}}
` + "```" + `
{{end}}

---

### 8. 输出要求

{{range $i, $c := .Constraints -}}
{{add $i 1}}. {{$c}}
{{end}}
`
