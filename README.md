# nep2midsence

> nep → midscene 自动化 Case 批量迁移工具

nep2midsence 是一个全屏交互式 Go CLI，通过多层次静态分析理解原始 nep case 的语义结构，生成结构化迁移指令，然后调起本地的 Coco（Claude Code CLI）执行实际代码改写，实现大批量 case 迁移的半自动化。

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

### 安装方式

提供三种安装方式，任选其一：

#### 方式一：`go install`（推荐）

直接安装到 `$GOPATH/bin`，通常已在系统 PATH 中：

```bash
go install github.com/sandaogouchen/nep2midsence/cmd/nep2midsence@latest
```

安装后直接运行 `nep2midsence` 即可进入全屏交互界面。

> 如果提示 `command not found`，请确保 `$GOPATH/bin` 在 PATH 中：
> ```bash
> # 查看 GOPATH/bin 路径
> echo "$(go env GOPATH)/bin"
>
> # 临时添加到 PATH
> export PATH="$PATH:$(go env GOPATH)/bin"
>
> # 永久添加（写入 shell 配置文件）
> echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
> source ~/.zshrc
> ```

#### 方式二：`make install`

从源码构建并安装到 `$GOPATH/bin`：

```bash
git clone https://github.com/sandaogouchen/nep2midsence.git
cd nep2midsence
make install
```

#### 方式三：从源码构建（手动管理）

构建二进制到当前目录，需要手动处理 PATH：

```bash
git clone https://github.com/sandaogouchen/nep2midsence.git
cd nep2midsence
make build
```

构建完成后，有两种运行方式：

```bash
# 方式 A：直接用相对路径运行
./nep2midsence

# 方式 B：复制到 PATH 中的目录
sudo cp ./nep2midsence /usr/local/bin/
# 或者
make install-global
```

### 验证安装

```bash
nep2midsence
```

预期输出：

```text
启动后进入全屏交互界面，底部输入栏可输入 slash command。
```

### 交互式使用

```bash
# 启动全屏 TUI
nep2midsence

# 指定配置文件启动
nep2midsence --config /path/to/.nep2midsence.yaml
```

启动后可使用以下命令：

| 命令 | 说明 |
|---|---|
| `/help` | 查看可用命令 |
| `/start` | 选择目标目录并执行完整流程：分析 → 生成 → 执行 → 验证 |
| `/analyze` | 选择目标目录并仅执行分析 |
| `/status` | 查看最近一次持久化运行状态 |
| `/history` | 查看历史运行记录 |
| `/config` | 查看当前配置 |
| `/config set <key> <value>` | 修改运行时配置 |
| `/config save` | 保存当前配置 |
| `/version` | 查看版本信息 |
| `/clear` | 清空当前会话日志 |
| `/quit` | 退出 TUI |

### 目录选择

`/start` 和 `/analyze` 都会先打开目录选择器。进入后可在顶部输入框直接模糊搜索目录，再用方向键和回车确认开始执行。

### 根级启动参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--config` | 配置文件路径 | `.nep2midsence.yaml` |
| `--verbose` | 详细日志输出 | false |

## 配置文件

在项目根目录创建 `.nep2midsence.yaml`：

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
  timeout: "3m"
  output_format: "json"
  allowed_tools:
    - "Read"
    - "Write"
    - "Edit"
    - "Bash"

execution:
  concurrency: 2
  retry_limit: 1
```

> **向后兼容**：工具也会自动识别旧的 `.casemover.yaml` 配置文件。
>
> `max_turns` 仍会被读取，但当前 `coco` CLI 不支持该参数，执行时不会再透传给 `coco`。

## 架构设计

```
┌────────────────────────────────────────────────────────────┐
│                  nep2midsence Full-screen TUI              │
├────────────┬───────────────┬───────────────┬───────────────┤
│  TUI 状态机  │  分析引擎      │  Prompt 生成器 │  Coco 调度器   │
│ (BubbleTea) │  (5层分析)     │  (模板+上下文)  │  (exec+并发)  │
├────────────┴───────────────┴───────────────┴───────────────┤
│                状态持久化 / 验证 / 报告模块                 │
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

## 常见问题

### `command not found` 怎么办？

这说明编译产物不在系统 PATH 中。解决方案：

1. **如果用 `make build` 构建的**：二进制在当前目录，用 `./nep2midsence` 运行，或执行 `make install` 安装到 PATH
2. **如果用 `go install` 安装的**：检查 `$GOPATH/bin` 是否在 PATH 中（见上方安装说明）
3. **快速验证**：运行 `which nep2midsence` 检查是否在 PATH 中

## License

MIT
