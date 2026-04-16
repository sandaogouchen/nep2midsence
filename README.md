# CaseMover

> nep → midscene 自动化 Case 批量迁移工具

CaseMover 是一个 Go CLI 工具，通过多层次静态分析理解原始 nep case 的语义结构，生成结构化迁移指令，然后调起本地的 Coco（Claude Code CLI）执行实际代码改写，实现大批量 case 迁移的半自动化。

## 核心特性

- **五层分析引擎**：AST 结构 → 调用链 → 数据流 → 模式识别 → 意图推断
- **API 指纹库**：nep API → midscene API 的确定性映射
- **结构化 Prompt 生成**：精确约束 AI 行为边界
- **Coco 调度器**：并发执行、失败重试、断点恢复
- **验证与报告**：编译检查、Diff 报告、迁移统计

## 快速开始

### 前置要求

- Go 1.21+
- [Claude Code CLI (coco)](https://github.com/anthropics/claude-code) 已安装并认证

### 安装

```bash
go install github.com/sandaogouchen/nep2midsence/cmd/casemover@latest
```

### 从源码构建

```bash
git clone https://github.com/sandaogouchen/nep2midsence.git
cd nep2midsence
go build -o casemover ./cmd/casemover/
```

### 使用

```bash
# 在项目根目录运行完整迁移流程
casemover start --dir ./tests/e2e/login

# 仅分析（dry-run）
casemover start --dir ./tests/e2e/login --dry-run

# 并发执行
casemover start --dir ./tests/e2e/ -j 4

# 仅执行分析，输出 JSON
casemover analyze --dir ./tests/e2e/ -o analysis.json

# 查看版本
casemover version
```

## 命令说明

| 命令 | 说明 |
|---|---|
| `start` | 完整流程：分析 → 生成 → 执行 → 验证 |
| `analyze` | 仅执行分析阶段 |
| `version` | 查看版本信息 |

### `start` 参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--dir` | 目标文件夹 | 交互选择 |
| `--dry-run` | 只生成计划不执行 | false |
| `-j, --jobs` | 并发数 | 1 |
| `--config` | 配置文件路径 | `.casemover.yaml` |
| `--verbose` | 详细日志 | false |

## 配置文件

在项目根目录创建 `.casemover.yaml`：

```yaml
source:
  framework: "nep"
  package_prefixes:
    - "github.com/xxx/nep"
  file_patterns:
    - "*_test.go"

target:
  framework: "midscene"
  output_dir: "midscene"

coco:
  max_turns: 15
  timeout: "3m"
  allowed_tools:
    - "Read"
    - "Write"
    - "Edit"
    - "Bash"

execution:
  concurrency: 2
  retry_limit: 1
```

## 架构设计

```
┌────────────────────────────────────────────────────────────┐
│                      CaseMover CLI                         │
├────────────┬───────────────┬───────────────┬───────────────┤
│  CLI 入口   │  分析引擎      │  Prompt 生成器 │  Coco 调度器   │
│  (cobra)   │  (5层分析)     │  (模板+上下文)  │  (exec+并发)  │
├────────────┴───────────────┴───────────────┴───────────────┤
│                     验证 & 报告模块                          │
└────────────────────────────────────────────────────────────┘
```

## 分析层说明

| 层 | 名称 | 功能 |
|---|---|---|
| L1 | AST 结构 | 解析代码语法树，提取函数、结构体、导入等 |
| L2 | 调用链 | 展开函数调用为线性操作序列 |
| L3 | 数据流 | 追踪选择器/URL/测试数据的来源 |
| L4 | 模式识别 | 识别 Page Object、数据驱动等模式 |
| L5 | 意图推断 | 推断操作的业务语义描述 |
| L6 | API 指纹 | nep → midscene 确定性 API 映射 |

## License

MIT
