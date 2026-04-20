<p align="center">
  <br>
  <code>nep2midsence</code>
  <br>
  <em>nep → midscene 自动化测试用例批量迁移引擎</em>
  <br><br>
  <a href="#快速开始">快速开始</a> · <a href="#工作原理">工作原理</a> · <a href="#交互命令">交互命令</a> · <a href="#配置参考">配置参考</a>
</p>

---

## 它能做什么

nep2midsence 是一个**全屏交互式 CLI**，用于将基于 nep 框架编写的 TypeScript E2E 测试用例**批量迁移**到 [Midscene](https://midscenejs.com/) 框架。

它不是简单的文本替换——而是一个**多层静态分析引擎 + AI 代码改写**的协作系统：

1. **理解**：通过 5 层分析流水线（AST → 调用链 → 数据流 → 模式识别 → 意图推断）深度理解每个测试用例的语义结构
2. **规划**：自动发现 Page Object、Helper、共享依赖之间的关系，按正确顺序编排迁移任务
3. **执行**：生成结构化 Prompt，调用本地 AI CLI（Coco / Claude Code / Codex）执行实际代码改写
4. **验证**：自动检查 NEP 残留标记和 TypeScript 编译错误，并发起修复循环

整个过程通过一个可滚动、有实时进度的终端 TUI 界面驱动。

## 核心能力

| 能力 | 说明 |
|---|---|
| **五层分析引擎** | L1 AST 解析 → L2 调用链构建 → L3 数据流追踪 → L4 模式识别（PageObject / ChainCall / DataDriven 等 7 种） → L5 意图推断 |
| **API 指纹映射** | 14 条内建映射规则（`nep.Click` → `ai.action('点击...')` 等），确定性转换 |
| **Helper 最小迁移** | 自动识别 Page Object / Wrapper 方法，仅迁移被 case 实际调用的方法子集 |
| **Barrel re-export 追踪** | 解析 `export * from` 链路，找到符号真实定义位置 |
| **跨仓库迁移** | 支持从源仓库分析、输出到不同的目标仓库 |
| **增量迁移** | 基于源文件 SHA256 哈希自动跳过未变更的文件（断点恢复 + 跨运行去重） |
| **NEP 残留自动修复** | 迁移后扫描目标文件中的 nep 标记残留，自动发起 AI 修复（最多 N 轮） |
| **编译失败自动修复** | 检测 TypeScript 编译错误并生成修复 Prompt（最多 N 轮） |
| **并发调度** | 信号量控制并发数，Helper 优先于 Case 执行 |

## 快速开始

### 前置要求

- **Go 1.21+**
- **Node.js**（用于 TypeScript AST 提取，可选 — 不可用时自动回退到正则方案）
- 以下 AI CLI 工具之一已安装并完成认证：
  - [Coco (Trae CLI)](https://github.com/anthropics/claude-code) — 默认
  - [Claude Code (`cc`)](https://github.com/anthropics/claude-code)
  - [Codex](https://github.com/openai/codex)

### 安装

**方式一：go install（推荐）**

```bash
go install github.com/sandaogouchen/nep2midsence/cmd/nep2midsence@latest
```

> 确保 `$GOPATH/bin` 在 PATH 中。可用 `echo "$(go env GOPATH)/bin"` 查看路径。

**方式二：从源码构建**

```bash
git clone https://github.com/sandaogouchen/nep2midsence.git
cd nep2midsence
make install    # 安装到 $GOPATH/bin
# 或
make build      # 构建到当前目录，用 ./nep2midsence 运行
```

### 启动

```bash
nep2midsence                                        # 进入全屏 TUI
nep2midsence --config /path/to/.nep2midsence.yaml   # 指定配置文件
nep2midsence --verbose                              # 详细日志模式
```

启动后你会看到全屏交互界面。底部输入栏接受 `/` 开头的斜杠命令。

## 交互命令

进入 TUI 后，所有操作通过斜杠命令完成：

| 命令 | 说明 |
|---|---|
| `/start` | 执行完整迁移流程：选择目录 → 分析 → 生成 → 执行 → 验证 → 修复 |
| `/analyze` | 仅执行分析流程，不触发 AI 改写 |
| `/status` | 查看最近一次运行的状态 |
| `/history` | 查看所有历史运行记录 |
| `/config` | 查看当前配置 |
| `/config set <key> <value>` | 修改运行时配置（如 `/config set execution.max_jobs 4`） |
| `/config save` | 将当前配置持久化到 YAML 文件 |
| `/model <coco\|cc\|codex>` | 切换 AI CLI 工具 |
| `/version` | 查看版本信息 |
| `/clear` | 清空当前日志 |
| `/quit` | 退出 |

**目录选择器**：`/start` 和 `/analyze` 会打开模糊搜索目录选择器。输入关键字筛选，方向键选择，回车确认。

**跨仓库模式**：`/start` 选择源目录后，会进入目标目录浏览器（Enter 进入子目录 / Tab 确认 / Esc 返回上级）。

## 工作原理

```
                    ┌─────────────────────────────────────────┐
                    │          nep2midsence TUI               │
                    │      全屏交互 · 实时进度 · 日志滚动       │
                    └────────────────┬────────────────────────┘
                                     │
               ┌─────────────────────┼─────────────────────┐
               ▼                     ▼                     ▼
    ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
    │   五层分析引擎     │  │  Prompt 生成器    │  │   AI CLI 调度器   │
    │                  │  │                  │  │                  │
    │  L1  AST 解析    │  │  结构化模板       │  │  Coco / CC / Codex│
    │  L2  调用链构建   │  │  API 映射表       │  │  并发执行 + 重试   │
    │  L3  数据流追踪   │  │  上下文约束       │  │  tmux TTY 包装    │
    │  L4  模式识别    │  │  修复/重试 Prompt  │  │  信号量控制       │
    │  L5  意图推断    │  │                  │  │                  │
    └──────────────────┘  └──────────────────┘  └──────────────────┘
               │                     │                     │
               └─────────────────────┼─────────────────────┘
                                     ▼
                    ┌─────────────────────────────────────────┐
                    │   验证 & 报告                            │
                    │   NEP 残留检查 · 编译检查 · Diff 生成     │
                    │   自动修复循环 · 迁移报告                  │
                    └─────────────────────────────────────────┘
                                     │
                                     ▼
                    ┌─────────────────────────────────────────┐
                    │   状态持久化                              │
                    │   .nep2midsence-state.json              │
                    │   断点恢复 · 跨运行去重 · 历史记录         │
                    └─────────────────────────────────────────┘
```

### 迁移流程详解

1. **Preflight** — 检查所选 AI CLI（coco/cc/codex）是否可用
2. **Scan & Analyze** — 扫描源目录，对每个匹配文件执行五层分析
3. **Plan** — 构建迁移计划：
   - 分离 helper/wrapper/dependency 任务和 case 任务
   - 解析 Page Object / 模块 / 组件依赖关系
   - 追踪 barrel re-export 符号链路
   - 识别 commonIt wrapper 并提取注入参数
   - 基于源文件哈希识别已迁移任务（跨运行去重）
4. **Generate** — 为每个任务生成结构化 Prompt，包含 API 映射表、操作步骤、代码模式、数据流、约束条件
5. **Execute** — 先并行执行 helpers/dependencies，再并行执行 cases
6. **Verify** — NEP 残留检查 + TypeScript 编译检查
7. **Fix** — NEP 残留修正循环 + 编译失败修复循环（最多 N 轮）
8. **Report** — 生成迁移报告（成功数 / 失败数 / 耗时 / Diff）

### API 映射示例

| nep API | Midscene 等价 | 备注 |
|---|---|---|
| `nep.Navigate(url)` | `page.goto(url)` | 直接映射 |
| `nep.FindElement("#login-btn")` | `ai.locator("登录按钮")` | CSS 选择器 → 自然语言 |
| `nep.Click(el)` | `ai.action('点击...')` | 需要意图重写 |
| `nep.SendKeys(el, text)` | `ai.action('在...输入...')` | 需要意图重写 |
| `nep.WaitForElement(sel)` | `ai.assert('...')` | 显式等待 → AI 断言 |
| `nep.GetText(el)` | `ai.query('获取...的文本')` | 信息检索 |
| `nep.Screenshot()` | `page.screenshot()` | 直接映射 |

## 配置参考

在项目根目录创建 `.nep2midsence.yaml`：

```yaml
source:
  framework: "nep"                    # 源框架标识
  language: "typescript"              # 源语言 (typescript | go)
  extensions:                         # 匹配的文件扩展名
    - ".ts"
  exclude:                            # 排除的目录
    - "node_modules"
    - "dist"
  package_prefixes:                   # nep 框架包前缀
    - "github.com/xxx/nep"
  file_patterns:                      # 文件匹配模式
    - "*_test.go"

target:
  framework: "midscene"               # 目标框架
  output_dir: "midscene"              # 输出子目录名
  base_dir: ""                        # 跨仓库目标根目录（可选）

execution:
  tool: "coco"                        # AI CLI 工具 (coco | cc | codex)
  max_jobs: 6                         # 最大并发数
  retry_limit: 3                      # 重试/修复循环次数
  timeout: "3m"                       # 单次执行超时

coco:
  output_format: "json"
  allowed_tools:                      # 允许 AI 使用的工具
    - "Read"
    - "Write"
    - "Edit"
    - "Bash"

analysis:
  max_call_depth: 5                   # 调用链展开最大深度

# wrapper/基础设施方法过滤规则
wrapper_filter:
  enable: true
  class_name_patterns: []
  method_name_patterns: []
```

所有配置项均可通过 TUI 中的 `/config set <dot.path> <value>` 动态修改。

## 项目结构

```
nep2midsence/
├── cmd/nep2midsence/          # CLI 入口
├── internal/
│   ├── analyzer/              # 五层分析引擎 (L1-L5) + TS 桥接
│   ├── cli/                   # 命令行参数解析、版本元数据
│   ├── config/                # 配置加载 / 持久化 / tsconfig 解析
│   ├── executor/              # AI CLI 执行器 (Coco/CC/Codex) + 并发调度器
│   ├── fingerprint/           # nep → midscene API 指纹映射库
│   ├── prompt/                # 结构化 Prompt 生成 (模板 + 知识文档)
│   ├── tui/                   # Bubble Tea 全屏 TUI (视图 + 运行时工作流)
│   ├── types/                 # 共享类型定义
│   └── verify/                # 验证器 + 报告生成器
├── scripts/                   # TypeScript AST 提取器 (Node.js)
├── docs/                      # 迁移指南、API 映射文档
├── testdata/                  # 测试数据
├── Makefile                   # 构建脚本
└── .nep2midsence.yaml         # 项目配置
```

## License

MIT
