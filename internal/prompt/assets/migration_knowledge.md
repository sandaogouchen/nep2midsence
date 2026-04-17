# NEP → Midscene 迁移速查手册（压缩版）

## 零、Midscene 标准模板（必读）

每个 Midscene case 的固定起手式：

```typescript
import { MidsceneCaseFunctionParams } from '@byted-midscene/pagepass-plugin';
import type { CaseFunctionParams } from '@pagepass/test';

describe('模块名', () => {
  it('case 名称', async (params: CaseFunctionParams<'test'>) => {
    const { midscene } = params as MidsceneCaseFunctionParams;
    const { agent } = midscene!;

    // 所有操作通过 agent 完成
    await agent.aiTap("点击'Sales'");
    await agent.aiInput("55", "'Daily'右侧的输入框");
    await agent.aiAssert("展示'Goal'元素");
  });
});
```

**关键三行（导入 → 解构 → 取 agent）**：

```typescript
import { MidsceneCaseFunctionParams } from '@byted-midscene/pagepass-plugin';
const { midscene } = params as MidsceneCaseFunctionParams;
const { agent } = midscene!;
```

> `agent` 是所有 AI 操作的唯一入口，替代 NEP 中的 `page.element()`、`ai?.action()`、`ai?.getElement()` 等全部方式。

---

## 一、NEP 三类操作模式

| 类型 | 示例 | 特征 |
|------|------|------|
| 纯 selector | `page.element('[data-testid="xxx"]').click()` | CSS 选择器直接操作 |
| 纯 AI | `ai?.action('点击[Campaign]')` / `ai?.getElement(prompt)` | 自然语言驱动 |
| selector + AI 兜底 | BaseComponent：先 `getElementBySelector()`，失败走 `ai?.action(prompt)` | 双路径，组件同时持有 `selector` 和 `prompt` |

**BaseComponent 关键字段**：每个组件有 `DEFAULT_SELECTOR`（CSS）+ `DEFAULT_PROMPT`（AI 描述）。

## 二、Midscene 代码形态

全部通过 `agent.aiXxx` 自然语言完成，无 selector：

```typescript
await agent.aiTap("点击'Sales'");
await agent.aiInput("55", "'Daily'右侧的输入框");
await agent.aiScroll('App', {scrollType: "singleAction", distance: 1200, direction: "down"});
await agent.aiWaitFor("展示'Goal'元素");
await agent.aiAssert("展示Congratulations或者这是广告列表页面");
```

## 三、迁移转换规则

### 规则 1：selector → AI

| NEP | Midscene | 要点 |
|-----|----------|------|
| `page.element('[selector]').click()` | `agent.aiTap("元素可视描述")` | 描述 UI 视觉特征，非 CSS |
| `page.element('[selector]').input('v')` | `agent.aiInput("v", "输入框描述")` | 参数序：(值, 定位) |
| `page.element({text:'xx'}).click()` | `agent.aiTap("点击'xx'")` | text 选择器最易翻译 |
| `.scrollIntoView()` | `agent.aiScroll(锚点, {scrollType, direction, distance})` | — |

### 规则 2：`ai?.action` → aiTap/aiInput/aiHover

| NEP | Midscene |
|-----|----------|
| `ai?.action('点击[xxx]')` | `agent.aiTap("点击'xxx'")` |
| `ai?.action('在xxx输入yyy')` | `agent.aiInput("yyy", "xxx输入框")` |
| `ai?.action('hover在【xxx】上')` | `agent.aiHover("xxx")` |
| `ai?.action('复合描述')` | `agent.aiTap("复合描述")` |
| `clickElementByVL(page, prompt)` | `agent.aiTap(prompt)` |

**核心差异**：NEP `ai?.action` 一句话含动作+目标；Midscene 动作在方法名（aiTap/aiInput），prompt 只描述"在哪里"。

### 规则 3：`ai?.getElement` + 原生操作 → 直接 AI

```typescript
// NEP                                          // Midscene
const el = await ai?.getElement('budget输入框');  → await agent.aiInput("21", "'Campaign budget'下方的输入框");
await el?.input('21');

const el = await ai?.getElement('xx', {skip});   → await agent.aiWaitFor("展示'xx'元素");
if (el?.isExisting()) break;
```

### 规则 4：scroll 循环找元素 → aiScroll / aiAction

```typescript
// NEP: wheel循环 + AI判断
for (let i=0; i<10; i++) { page.mouse.wheel({deltaY:1200}); if(ai?.getElement(xx)) break; }

// Midscene A: 单次滚动
await agent.aiScroll('锚点', {scrollType:"singleAction", distance:1200, direction:"down"});
// Midscene B: AI自主滚动
await agent.aiAction("向下滚动页面直到看到 Campaign budget");
```

### 规则 5：selector 兜底 AI → 只保留 AI 路径

```typescript
// NEP 双路径                    // Midscene 单路径
if(selector找到) hover();       → await agent.aiHover(`列表中的'${name}'文本`);
else ai?.getElement→hover();
```

### 规则 6：BaseComponent → 提取 prompt

找到组件类 → 取 `DEFAULT_PROMPT` → 作为 Midscene prompt。无 prompt 只有 selector 时需手写。

### 规则 7：等待策略

| NEP | Midscene | 说明 |
|-----|----------|------|
| `waitForPageLoadStable(page)` | `waitForPageLoadStable(page)` | **直接复用** |
| `page.wait(5000)` | `sleep(5000)` | 尽量用 aiWaitFor 替代 |
| `page.waitForResponse('/api/xx')` | 无直接等效 | 需用 page 原生 API |
| `ai?.getElement(p, {skipUndefined})` 判断存在 | `agent.aiWaitFor("展示'xx'")` | — |

### 规则 8：iframe

`page.switchToRootFrame()` 可直接复用，agent 在当前 frame context 操作。

### Prompt 风格对照

NEP `【】`/`[]` → Midscene `''`（单引号），语义保持一致即可。

## 四、Midscene Agent API 速查

### 动作类

| 方法 | 签名 | 说明 |
|------|------|------|
| `aiTap` | `(prompt, opt?)` | 点击。opt 可含 `fileChooserAccept` |
| `aiRightClick` | `(prompt, opt?)` | 右键 |
| `aiDoubleClick` | `(prompt, opt?)` | 双击 |
| `aiHover` | `(prompt, opt?)` | 悬停 |
| `aiInput` | 推荐：`(prompt, {value, mode?})` / 兼容旧：`(value, prompt, opt?)` | mode: replace/clear/typeOnly/append(→typeOnly) |
| `aiKeyboardPress` | 推荐：`(prompt, {keyName})` / 旧：`(keyName, prompt?)` | 按键 |
| `aiScroll` | 推荐：`(prompt, {scrollType, direction, distance})` / 旧：`(scrollParam, prompt?)` | scrollType: singleAction/scrollToBottom |
| `aiPinch` | `(prompt, {direction, distance?, duration?})` | 缩放 |

### 多步执行

| 方法 | 说明 |
|------|------|
| `aiAct(task, opt?)` | **推荐**。AI 自主规划多步。opt: cacheable, deepThink, deepLocate |
| `aiAction` | **废弃别名** = aiAct |
| `ai` | 别名 = aiAct |

### 信息提取

| 方法 | 返回 |
|------|------|
| `aiQuery(demand, opt?)` | `any`（按 demand 结构返回） |
| `aiBoolean / aiNumber / aiString / aiAsk` | 对应类型 |

默认：`domIncluded: false, screenshotIncluded: true`

### 定位与校验

| 方法 | 说明 |
|------|------|
| `aiLocate(prompt, opt?)` | 返回 `{rect, center, dpr?}` 不执行操作 |
| `describeElementAtPoint(center, opt?)` | 坐标→prompt 描述 |
| `verifyLocator(prompt, opt, center, verifyOpt?)` | 验证 prompt 定位准确性 |

### 断言与等待

| 方法 | 行为 |
|------|------|
| `aiAssert(assertion, msg?, opt?)` | 默认失败 throw；`keepRawResponse:true` 返回 `{pass, thought, message}` |
| `aiWaitFor(assertion, opt?)` | 默认 timeout 15s，间隔 3s |

### 其他

| 方法 | 说明 |
|------|------|
| `runYaml(yaml)` | 执行 YAML 流程脚本 |
| `evaluateJavaScript(script)` | 底层需支持 |
| `recordToReport(title?, opt?)` | 截屏写报告 |
| `freezePageContext / unfreezePageContext` | 冻结/解冻 UI context |
| `flushCache({cleanUnused?})` | 刷新缓存（未配置会 throw） |
| `destroy()` | 销毁 agent |

## 五、封装函数迁移策略（新建重写，不替换原函数）

当 case 中调用了封装函数（如 `listPage.commonActions.editCampaign2()`、pageObject 方法等），**不要直接修改原有封装函数**，而是采用"新建重写函数"策略：

### 步骤

1. **新建函数**：在封装文件中新增一个带 `Midscene` 后缀的新函数
   ```typescript
   // 原函数保持不变
   async editCampaign2(page: Page, ai: AI, campaignName: string) {
     // ... 原有 NEP 实现 ...
   }

   // 新建 Midscene 版本
   async editCampaign2Midscene(agent: AgentWI, campaignName: string) {
     await agent.aiTap(`列表中名为'${campaignName}'的campaign的编辑按钮`);
     // ... 用 midscene agent API 重写完整逻辑 ...
   }
   ```

2. **保持原函数不变**：原有的 `editCampaign2()` 函数完全不动，其他未迁移的 case 仍然可以正常使用

3. **case 中切换调用**：迁移后的 case 文件中，将调用改为新函数
   ```typescript
   // 迁移前
   await listPage.commonActions.editCampaign2(page, ai, campaignName);
   // 迁移后
   await listPage.commonActions.editCampaign2Midscene(agent, campaignName);
   ```

4. **传入 agent**：新函数需要接收 `agent` 参数（由 case 传入），而非原来的 `page`/`ai` 参数

5. **避免重复**：若封装文件中已存在 `xxxMidscene` 版本函数，case 直接调用即可，无需再次新建

---

## 六、迁移 Checklist

1. **模板**：用上面「第零节」的标准 describe/it + 三行导入解构，取到 `agent`
2. **逐行翻译**：selector→AI prompt / `ai?.action`→拆 aiTap/aiInput / `ai?.getElement`+操作→合并 / 双路径→只留 AI
3. **等待**：`waitForPageLoadStable` 复用；硬等待改 `aiWaitFor`
4. **iframe**：保留 `switchToRootFrame()`
5. **before/after**：cookie/mock/清理照搬
6. **断言**：接口校验评估是否用 `aiAssert` 替代
