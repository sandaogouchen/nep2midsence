# Midscene Agent (Pagepass) 使用文档

本文档基于当前仓库 `node_modules` 中的源码与类型声明整理，覆盖你在 Pagepass 测试用例里拿到的 `midscene.agent` 的**完整方法集合、参数含义、默认行为、废弃接口与推荐写法**。


## 4. Agent 方法全集（按用途分组）

### 4.1 基于定位的动作（Click / Input / Scroll 等）

#### aiTap

```ts
aiTap(locatePrompt: TUserPrompt, opt?: LocateOption & {
  fileChooserAccept?: string | string[];
}): Promise<any>
```

- 作用：AI 定位元素后点击。
- `fileChooserAccept`：当点击会触发文件选择器时，用于自动处理文件选择（会被归一化为绝对路径并校验存在性）。

源码：[@midscene/core/dist/lib/agent/agent.js](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@midscene/core/dist/lib/agent/agent.js)

#### aiRightClick / aiDoubleClick / aiHover

```ts
aiRightClick(locatePrompt: TUserPrompt, opt?: LocateOption): Promise<any>
aiDoubleClick(locatePrompt: TUserPrompt, opt?: LocateOption): Promise<any>
aiHover(locatePrompt: TUserPrompt, opt?: LocateOption): Promise<any>
```

#### aiInput（推荐签名 + 兼容旧签名）

推荐写法（把 `value` 放进 opt）：

```ts
aiInput(locatePrompt: TUserPrompt, opt: LocateOption & {
  value: string | number;
  autoDismissKeyboard?: boolean;
  mode?: "replace" | "clear" | "typeOnly" | "append";
}): Promise<any>
```

兼容旧写法（仓库内大量使用）：

```ts
aiInput(value: string | number, locatePrompt: TUserPrompt, opt?: LocateOption & {
  autoDismissKeyboard?: boolean;
  mode?: "replace" | "clear" | "typeOnly" | "append";
}): Promise<any>
```

重要实现细节：
- `value` 会被转成字符串（number => String）。
- `mode: "append"` 在实现里会被转换成 `"typeOnly"`（避免做 clear/replace）。

源码：[@midscene/core/dist/lib/agent/agent.js](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@midscene/core/dist/lib/agent/agent.js)

#### aiKeyboardPress（推荐签名 + 兼容旧签名）

推荐写法：

```ts
aiKeyboardPress(locatePrompt: TUserPrompt, opt: LocateOption & {
  keyName: string;
}): Promise<any>
```

兼容旧写法：

```ts
aiKeyboardPress(keyName: string, locatePrompt?: TUserPrompt, opt?: LocateOption): Promise<any>
```

#### aiScroll（推荐签名 + 兼容旧签名）

推荐写法：

```ts
aiScroll(locatePrompt: TUserPrompt | undefined, opt: LocateOption & ScrollParam): Promise<any>
```

兼容旧写法：

```ts
aiScroll(scrollParam: ScrollParam, locatePrompt?: TUserPrompt, opt?: LocateOption): Promise<any>
```

重要实现细节：
- legacy `scrollType` 会被自动归一化：`once -> singleAction`，`untilBottom -> scrollToBottom` 等。

#### aiPinch

```ts
aiPinch(locatePrompt: TUserPrompt | undefined, opt: LocateOption & {
  direction: "in" | "out";
  distance?: number;
  duration?: number;
}): Promise<any>
```

### 4.2 规划式多步执行（让 AI 自己完成一段任务）

#### aiAct（推荐） / aiAction（废弃别名） / ai（别名）

```ts
aiAct(taskPrompt: string, opt?: AiActOptions): Promise<string | undefined>

// deprecated: aiAction == aiAct
aiAction(taskPrompt: string, opt?: AiActOptions): Promise<string | undefined>

// alias: ai == aiAct
ai(...args: Parameters<this["aiAct"]>): Promise<string | undefined>
```

重要实现细节（来自源码）：
- `abortSignal`：如果一开始已 aborted 会直接 throw。
- `cacheable`：允许复用计划缓存；当命中缓存且缓存里有 yaml workflow 时，会走 `runYaml()` 执行。
- `deepThink`：`"unset"` 会被当成未设置；`deepThink` 会影响规划时包含图片数量等。

源码：[@midscene/core/dist/lib/agent/agent.js](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@midscene/core/dist/lib/agent/agent.js)

### 4.3 信息提取 / 判断 / 问答

#### aiQuery

```ts
aiQuery<ReturnType = any>(
  demand: string | Record<string, string>,
  opt?: ServiceExtractOption
): Promise<ReturnType>
```

#### aiBoolean / aiNumber / aiString / aiAsk

```ts
aiBoolean(prompt: TUserPrompt, opt?: ServiceExtractOption): Promise<boolean>
aiNumber(prompt: TUserPrompt, opt?: ServiceExtractOption): Promise<number>
aiString(prompt: TUserPrompt, opt?: ServiceExtractOption): Promise<string>
aiAsk(prompt: TUserPrompt, opt?: ServiceExtractOption): Promise<string> // 实现上等价 aiString
```

默认提取选项（源码）：`domIncluded: false, screenshotIncluded: true`  
来源：[@midscene/core/dist/lib/agent/agent.js](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@midscene/core/dist/lib/agent/agent.js)

### 4.4 定位与校验（不执行点击）

#### aiLocate

```ts
aiLocate(prompt: TUserPrompt, opt?: LocateOption): Promise<{
  rect: any;
  center: [number, number];
  dpr?: number;
}>
```

说明：
- 类型声明里返回的是 `{ rect, center }`（更保守）；源码里额外返回 `dpr` 字段。

#### describeElementAtPoint / verifyLocator

```ts
describeElementAtPoint(
  center: [number, number],
  opt?: {
    verifyPrompt?: boolean;
    retryLimit?: number;
    deepLocate?: boolean;
  } & { centerDistanceThreshold?: number }
): Promise<{
  prompt: string;
  deepLocate: boolean;
  verifyResult?: any;
}>

verifyLocator(
  prompt: string,
  locateOpt: LocateOption | undefined,
  expectCenter: [number, number],
  verifyLocateOption?: { centerDistanceThreshold?: number }
): Promise<{ pass: boolean; rect: any; center: [number, number]; centerDistance?: number }>
```

用途：把屏幕坐标点“描述成可复用的 prompt”，并验证 prompt 是否能稳定定位到同一位置。

### 4.5 断言与等待

#### aiAssert

```ts
aiAssert(
  assertion: TUserPrompt,
  msg?: string,
  opt?: { keepRawResponse?: boolean } & ServiceExtractOption
): Promise<{ pass: boolean; thought?: string; message?: string } | undefined>
```

重要行为（源码）：
- 默认**失败会 throw**（抛出带 reason 的 Error）。
- 只有当 `keepRawResponse: true` 时，失败不会 throw，而是返回 `{ pass: false, thought, message }`。

源码：[@midscene/core/dist/lib/agent/agent.js](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@midscene/core/dist/lib/agent/agent.js)

#### aiWaitFor

```ts
aiWaitFor(assertion: TUserPrompt, opt?: AgentWaitForOpt): Promise<void>
```

默认值（源码）：
- `timeoutMs: 15000`
- `checkIntervalMs: 3000`

### 4.6 YAML/脚本/报告/生命周期

#### runYaml

```ts
runYaml(yamlScriptContent: string): Promise<{ result: Record<string, any> }>
```

说明：可执行一段 Midscene YAML 流程脚本。失败会聚合 error 后 throw。

#### evaluateJavaScript

```ts
evaluateJavaScript(script: string): Promise<any>
```

注意：如果当前 agent 的底层 interface 不支持 `evaluateJavaScript` 会直接 assert 失败。

#### recordToReport / logScreenshot（废弃）

```ts
recordToReport(title?: string, opt?: { content: string }): Promise<void>

// deprecated: logScreenshot == recordToReport
logScreenshot(title?: string, opt?: { content: string }): Promise<void>
```

实现上会截屏并追加到报告 dump 中，然后 flush 报告。

#### dump listeners

```ts
addDumpUpdateListener(listener: (dump: string, executionDump?: any) => void): () => void
removeDumpUpdateListener(listener: (dump: string, executionDump?: any) => void): void
clearDumpUpdateListeners(): void
```

#### freezePageContext / unfreezePageContext

```ts
freezePageContext(): Promise<void>
unfreezePageContext(): Promise<void>
```

用途：冻结 UI context，减少后续每次 AI 操作重复解析页面的开销（适合短时间内连续操作）。

#### flushCache

```ts
flushCache(options?: { cleanUnused?: boolean }): Promise<void>
```

注意：如果未配置 cache，会直接 throw：`Cache is not configured`。

#### destroy

```ts
destroy(): Promise<void>
```

用途：flush + finalize 报告，销毁底层 interface，重置 dump。

## 5. 推荐用法与示例（TypeScript）

### 5.1 从 Pagepass params 取 agent

```ts
import { MidsceneCaseFunctionParams } from "@byted-midscene/pagepass-plugin";
import type { CaseFunctionParams } from "@pagepass/test";

it("case name", async (params: CaseFunctionParams<"test">) => {
  const { midscene } = params as MidsceneCaseFunctionParams;
  const { agent } = midscene;

  await agent.aiTap("点击 'Sales'");
});
```

### 5.2 输入：用推荐签名（避免旧重载歧义）

```ts
await agent.aiInput("'Daily'右侧的输入框", { value: "55" });
// 追加输入（实现会转成 typeOnly）
await agent.aiInput("Text 输入框", { value: "hello", mode: "append" });
```

### 5.3 滚动：兼容 legacy scrollType，但建议用新值

```ts
// 推荐
await agent.aiScroll("Products", { scrollType: "singleAction", direction: "down", distance: 1000 });

// legacy，内部会自动归一化
await agent.aiScroll("Products", { scrollType: "once", direction: "down", distance: 1000 });
```

### 5.4 断言：想拿到 thought/message 就用 keepRawResponse

```ts
// 默认：失败会 throw
await agent.aiAssert("页面展示 'Ad name' 元素");

// 不 throw，返回结构化结果
const res = await agent.aiAssert("页面展示 'Ad name' 元素", undefined, { keepRawResponse: true });
if (res && !res.pass) {
  console.log(res.thought, res.message);
}
```

### 5.5 多步任务：用 aiAct（不要再新增 aiAction）

```ts
await agent.aiAct("完成登录并进入 Create 页面", {
  cacheable: true,
  deepLocate: true,
});
```

### 5.6 文件选择器：fileChooserAccept

```ts
await agent.aiTap("点击上传按钮", {
  fileChooserAccept: ["/abs/path/to/1.png", "/abs/path/to/2.png"],
});
```

注意：路径会被强制转成绝对路径并校验文件存在，否则会 throw。

## 6. 本仓库用例里的使用现状（统计）

以下统计来自对 `e2e/**/*.ts` 的扫描（仅用于反映“常用程度”，不代表能力全集）：

- `aiTap`: 378 次 / 30 个文件
- `aiScroll`: 117 次 / 23 个文件
- `aiInput`: 54 次 / 27 个文件（其中大量是旧重载签名）
- `aiWaitFor`: 46 次 / 24 个文件
- `aiAssert`: 52 次 / 29 个文件
- `aiQuery`: 19 次 / 19 个文件
- `aiAction`: 26 次 / 12 个文件（建议逐步替换为 `aiAct`）

## 7. 版本与溯源

本文档主要溯源文件（建议排查行为差异时直接看这里）：

- `Agent` 类型：[@midscene/core/dist/types/agent/agent.d.ts](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@midscene/core/dist/types/agent/agent.d.ts)
- `Agent` 实现：[@midscene/core/dist/lib/agent/agent.js](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@midscene/core/dist/lib/agent/agent.js)
- Locate 参数归一化：[@midscene/core/dist/lib/yaml/utils.js](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@midscene/core/dist/lib/yaml/utils.js)
- Pagepass 注入：[@byted-midscene/pagepass-plugin/dist/index.js](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@byted-midscene/pagepass-plugin/dist/index.js)
- Web 导出：[@midscene/web/dist/types/index.d.ts](file:///Users/bytedance/tt4b_ai_e2e/node_modules/@midscene/web/dist/types/index.d.ts)

