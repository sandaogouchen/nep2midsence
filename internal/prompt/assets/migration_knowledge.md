# NEP → Midscene 迁移速查手册（压缩版）

## 零、Midscene 标准模板（必读）

每个 Midscene case 的固定起手式：

```typescript
import { MidsceneCaseFunctionParams } from '@byted-midscene/pagepass-plugin';
import type { CaseFunctionParams } from '@pagepass/test';

describe('vv_cbo_standard6s_cta', () => {
  before(async ({ page }) => {
    await 原 case 中的 before 注入函数(page);
  });
  before(commonBeforeForBrandAuction);

  it('case 名称', async (params: CaseFunctionParams<'test'>) => {
    const { page } = params;
    const { midscene } = params as MidsceneCaseFunctionParams;
    const { agent } = midscene!;

    // 起手必须先复用原 case 的 before 链路，完成环境、cookie、权限、advId 注入
    // 不要把 goToCreatePage() 当成固定模板；页面进入方式沿用原 case 即可
    await page.goto('原 case 实际访问的页面或入口链接');

    // 所有交互统一通过 agent 完成
    await agent.aiTap("点击'Sales'");
    await agent.aiInput("55", "'Daily'右侧的输入框");
    await agent.aiAssert("展示'Goal'元素");
  });
});
```

**关键起手式（导入 → before 注入 → 解构 → 取 agent）**：

```typescript
import { MidsceneCaseFunctionParams } from '@byted-midscene/pagepass-plugin';
before(async ({ page }) => {
  await 原 case 中的 before 注入函数(page);
});
before(commonBeforeForBrandAuction);
const { midscene } = params as MidsceneCaseFunctionParams;
const { agent } = midscene!;
```

> `agent` 是所有 AI 操作的唯一入口，替代 NEP 中的 `page.element()`、`ai?.action()`、`ai?.getElement()` 等全部方式。

> `before` 链路不要删。像 `commonBeforeForBrandAuction` 这类封装，本质上会继续走 `commonBefore(..., EnumAdvIDInfo.ADV_Brand_Auction)`，里面会执行 `setEnv(page)`、`setCookies(page, advInfo?.userId)`、`mockPermission(page, advInfo)`、`mockUnSelectReco(page, advInfo?.advId)` 和窗口尺寸设置；这部分就是当前页面跳转前所依赖的 cookie / 用户 / 广告主上下文。

> case 级 `before` 也要保留，但不要把函数名写死。agent 必须回到原始 case 代码里查当前 case 实际用了哪个 `before(async ({ page }) => ...)` 注入函数，并原样继承到 Midscene case 中。它通常负责在进入页面前补齐接口拦截，例如：

```typescript
export const XxxActionCreation = (page: Page) => {
  return page.intercept(
    '某个原 case 依赖的接口',
    MockResponse,
  );
};
```

> 也就是说，agent 的固定动作应该是：先打开原始 case 文件，找到 `describe` 里的全部 `before(...)` / `beforeEach(...)`，识别出页面跳转前依赖的注入函数、公共 `commonBeforeXxx` 封装、以及对应的 `EnumAdvIDInfo`，再把这套上下文迁移到 Midscene case；不要臆造一个通用的 `BrandActionCreation`。

> 对应账号信息如果原 case 使用 `EnumAdvIDInfo.ADV_Brand_Auction`，则沿用这套 advertiser 上下文：

```typescript
ADV_Brand_Auction: {
  advId: '7594306930307383297',
  userId: '7594306459032159243',
},
```

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

## 5.5 commonIt Wrapper 迁移规则

### 什么是 commonIt

`commonIt` 是 NEP E2E 测试中常见的测试封装函数，它包装了标准 `it`/`test`，自动完成：

1. **URL 导航**：自动跳转到指定测试页面
2. **Page Object 注入**：自动实例化并注入业务 Page Object（如 `listPage`、`createPage`）
3. **参数解构**：回调函数通过参数解构接收 `{ page, ai, listPage, caseParams, ... }`

```typescript
// NEP 中的 commonIt 用法
commonIt('创建广告', { url: '/campaign/create', pageObjects: [ListPage, CreatePage] }, 
  async ({ page, ai, listPage, createPage, caseParams }) => {
    // page, ai, listPage, createPage 都是 wrapper 自动注入的
    await listPage.commonActions.clickCreate();
    await ai?.action('点击[Continue]');
  }
);
```

### 迁移规则

将 `commonIt` 迁移到标准 Midscene `it`/`test` 时：

1. **替换 wrapper**：`commonIt(...)` → `it(...)` 或 `test(...)`
2. **回调签名**：改为 `CaseFunctionParams<'test'>`，仅接收 `{ page, midscene }`
3. **显式导航**：wrapper 自动执行的 URL 跳转需在 test body 开头手动补上：
   ```typescript
   await page.goto('/campaign/create');
   ```
4. **Page Object 实例化**：wrapper 注入的 Page Object 需自行创建：
   ```typescript
   const listPage = new ListPage(page);
   const createPage = new CreatePage(page);
   ```
5. **删除 wrapper import**：移除对 `commonIt` 的 import 声明
6. **识别假依赖**：`commonIt` 回调参数中解构出的变量（如 `listPage`、`caseParams`）是 wrapper 在运行时注入的，**不是真实的模块 import**。迁移时不要试图从其他文件 import 这些变量，而应按上述规则自行实例化或替换

### 迁移示例

**迁移前（NEP + commonIt）**：
```typescript
import { commonIt } from '@e2e/common';
import { ListPage } from '@pages/ListPage';

describe('Campaign', () => {
  commonIt('编辑广告', { url: '/campaign/list' }, 
    async ({ page, ai, listPage, caseParams }) => {
      await listPage.commonActions.editCampaign(page, ai, caseParams.name);
      await ai?.action('点击[Save]');
    }
  );
});
```

**迁移后（Midscene 标准写法）**：
```typescript
import { MidsceneCaseFunctionParams } from '@byted-midscene/pagepass-plugin';
import type { CaseFunctionParams } from '@pagepass/test';
import { ListPage } from '@pages/ListPage';

describe('Campaign', () => {
  it('编辑广告', async (params: CaseFunctionParams<'test'>) => {
    const { page } = params;
    const { midscene } = params as MidsceneCaseFunctionParams;
    const { agent } = midscene!;

    // 1. 显式导航（原 wrapper 自动完成）
    await page.goto('/campaign/list');

    // 2. 手动实例化 Page Object（原 wrapper 自动注入）
    const listPage = new ListPage(page);

    // 3. 业务逻辑迁移
    await listPage.commonActions.editCampaignMidscene(agent, caseParams.name);
    await agent.aiTap("点击'Save'");
  });
});
```

---

## 六、迁移 Checklist

1. **模板**：用上面「第零节」的标准 describe/it + 三行导入解构，取到 `agent`
2. **逐行翻译**：selector→AI prompt / `ai?.action`→拆 aiTap/aiInput / `ai?.getElement`+操作→合并 / 双路径→只留 AI
3. **等待**：`waitForPageLoadStable` 复用；硬等待改 `aiWaitFor`
4. **iframe**：保留 `switchToRootFrame()`
5. **before/after**：cookie/mock/清理照搬
6. **断言**：接口校验评估是否用 `aiAssert` 替代
