# NEP → Midscene 框架迁移经验知识


## 一、NEP case 的真实代码形态

NEP case 的核心特征是 **原生 selector 操作和 AI 操作（`ai?.action` / `ai?.getElement`）混合使用**，而不是层层封装。以下是从实际代码中提取的操作模式。

### 1.1 NEP case 中的三类操作

**第一类：纯原生 selector 操作**

```typescript
// 直接用 CSS selector 定位并操作
await page.element('[class="input-content--BfVKZ KsInput"]').input(campaignId);
await page.element('[class="input-content--BfVKZ KsInput"]').click();
await page.element('[class="filterPanelItem--K9gUu rcc-cursor-pointer KsText"]').click();
await listPage.editAdSubmitBtn.click();  // 底层也是 selector：[data-testid="common_next_button"]
```

**第二类：纯 AI 操作**

```typescript
// 直接用自然语言 prompt 驱动 AI 操作
await ai?.action('点击页面左侧的[Campaign]文本');
await ai?.action(`将鼠标hover在【${campaignName}】上`);
await ai?.action(`点击[${campaignName}]下方带铅笔图标的[Edit]按钮`);
await ai?.action('在Number of copies下面的输入框输入【1】');
```

**第三类：selector 优先 + AI fallback（BaseComponent 模式）**

```typescript
// BaseComponent.click() 的底层逻辑：
async click(aiOptions?: AiActionOptions): Promise<void> {
    await waitForPageLoadStable(this.page);
    // 第一步：尝试 selector
    const element = await this.getElementBySelector();
    if (element) {
        await element.click();
        return;
    }
    // 第二步：selector 失败，走 AI
    await this.ai?.action(`点击 ${this.prompt}`, aiOptions);
}

// BaseComponent.type() 同理：
async type(text: string, aiOptions?: AiActionOptions): Promise<void> {
    await waitForPageLoadStable(this.page);
    const element = await this.getElementBySelector();
    if (element) {
        await element.scrollIntoView();
        await element.click({ count: 3 });
        await this.page.keyboard.insertText(text);
        return;
    }
    await this.ai?.action(`在${this.prompt} 输入 ${text}`, aiOptions);
}
```

每个 BaseComponent 组件同时持有 `selector`（CSS 选择器）和 `prompt`（AI 描述），例如：
```typescript
// EditAdSubmitBtn
static DEFAULT_SELECTOR: string = '[data-testid="common_next_button"]';
static DEFAULT_PROMPT: string = '[Submit]按钮';
```

### 1.2 NEP case 中 AI 操作的具体用法

从 `reach-edit-campaign.ts` 及其依赖中提取的真实 AI 调用：

| AI 调用方式 | 示例 | 用途 |
|-----------|------|------|
| `ai?.getElement(prompt)` | `ai?.getElement('页面中的【Campaign budget】字样', {skipUndefined: true})` | 获取元素引用，用于后续判断是否存在、获取坐标等 |
| `ai?.getElement(prompt)` + 操作 | `const el = await ai?.getElement('在 [Campaign budget] 下方的文本框'); await el?.input('21');` | 获取元素后调用原生 input/click |
| `ai?.action(prompt)` | `ai?.action('点击页面左侧的[Campaign]文本')` | 直接让 AI 完成一步操作（定位+动作） |
| `ai?.action(prompt)` + hover | `ai?.action('将鼠标hover在【xxx】上')` | AI 做 hover 操作 |
| `clickElementByVL(page, prompt)` | `clickElementByVL(page, '点击页面左侧的[Campaign]文本')` | VL 视觉模型定位坐标 → `page.mouse.click({x, y})` |

### 1.3 NEP case 中 selector 和 AI 混合的典型场景

**场景 A：同一个操作流程中交替使用**

```typescript
// reach-edit-campaign.ts 中的实际代码：
// 步骤1：AI 操作 —— 点击 Campaign tab + 搜索
await clickElementByVL(this.page, '点击页面左侧的[Campaign]文本');
// 步骤2：selector 操作 —— 搜索框输入（selector 可靠时直接用）
await this.page.element('[class="input-content--BfVKZ KsInput"]').input(campaignId);
// 步骤3：AI 操作 —— hover 到目标行（列表行无固定 selector）
await this.ai?.action(`将鼠标hover在【${campaignName}】上`);
// 步骤4：AI 操作 —— 点击 Edit 按钮（浮现按钮无固定 selector）
await this.ai?.action(`点击[${campaignName}]下方带铅笔图标的[Edit]按钮`);
```

**场景 B：scroll 循环找元素**

```typescript
// NEP 用 原生 wheel + AI getElement 循环找元素
const MaxRetry = 10;
for (let i = 0; i < MaxRetry; i++) {
    await page.mouse.wheel({ deltaX: 0, deltaY: 1200 });
    const bidControl = await ai?.getElement('页面中的【Campaign budget】字样', { skipUndefined: true });
    if (bidControl?.isExisting()) {
        break;
    }
}
```

**场景 C：AI getElement 获取后走原生操作**

```typescript
// AI 找元素 → 原生 scrollIntoView + input
const budgetInput = await ai?.getElement('在 [Campaign budget] 下方的文本框');
await budgetInput?.scrollIntoView();
await budgetInput?.input('21');
```

**场景 D：selector 找不到时 AI 兜底（CommonActions 中高频模式）**

```typescript
// 先尝试 CSS 遍历行，找到了用原生 hover
const findElement = await this.findElementInRows(campaignName);
if (!findElement) {
    // 找不到就走 AI
    let campaign = await this.ai?.getElement(`列表中的[${campaignName}]文本`, { skipUndefined: true });
    if (!campaign) {
        campaign = await this.ai?.getElement(`Name下面第一个的图标`, { skipUndefined: true });
    }
    await campaign?.hover();
} else {
    await findElement.hover();
}
```

---

## 二、Midscene case 的代码形态

Midscene 完全去掉了 selector，**所有操作通过 `agent.aiXxx` 自然语言完成**：

```typescript
const { agent } = (params as MidsceneCaseFunctionParams).midscene!;

await agent.aiTap("点击 'Sales'");
await agent.aiTap("点击'Got it'按钮");
await agent.aiTap("点击'Website'下方的'App'");
await agent.aiScroll('App', {scrollType: "singleAction", distance: 1200, direction: "down"});
await agent.aiTap("点击'Campaign budget'元素");
await agent.aiInput("55", "'Daily'右侧的输入框");
await agent.aiTap("点击Continue按钮");
await agent.aiWaitFor("展示'Goal'元素");
// ... 更多 aiTap / aiInput / aiScroll
await agent.aiAssert("展示Congratulations或者这是TikTok Ads Manager的广告列表页面");
```

---

## 三、迁移转换规则（Agent 可直接执行）

### 规则 1：NEP 原生 selector 操作 → Midscene AI 操作

| NEP 写法 | Midscene 写法 | 说明 |
|----------|-------------|------|
| `page.element('[data-testid="xxx"]').click()` | `agent.aiTap("该元素的可视文本描述")` | 需要根据 selector 对应的 UI 元素写出自然语言描述 |
| `page.element('[data-testid="xxx"]').input('value')` | `agent.aiInput("value", "该输入框的可视文本描述")` | 注意 midscene aiInput 参数顺序是 (值, 定位描述) |
| `page.element({css: 'xxx'}).scrollIntoView()` | `agent.aiScroll('锚点元素', {scrollType, direction, distance})` | 或者放在 aiTap 之前让 AI 自行处理可见性 |
| `page.element({text: 'xxx'}).click()` | `agent.aiTap("点击'xxx'")` | text 选择器最容易翻译，文本即 prompt |

**关键点**：selector 转 AI prompt 时，不能写 CSS 选择器本身，而是要描述这个元素在 UI 上的**视觉特征**：它旁边/上方/下方有什么文字、它是什么类型的控件（按钮/输入框/下拉框/开关）。

### 规则 2：NEP `ai?.action(prompt)` → Midscene `agent.aiTap/aiInput/aiAction`

| NEP AI 写法 | Midscene 写法 | 说明 |
|------------|-------------|------|
| `ai?.action('点击[xxx]文本')` | `agent.aiTap("点击'xxx'")` | 纯点击用 aiTap |
| `ai?.action('在xxx输入框输入yyy')` | `agent.aiInput("yyy", "xxx输入框")` | 输入拆成值和定位两参数 |
| `ai?.action('将鼠标hover在【xxx】上')` | `agent.aiHover("xxx")` | hover 操作 |
| `ai?.action('点击xxx下方带铅笔图标的[Edit]按钮')` | `agent.aiTap("点击xxx下方带铅笔图标的Edit按钮")` | 复合描述直接作为 prompt |
| `clickElementByVL(page, prompt)` | `agent.aiTap(prompt)` | VL 视觉定位 → Midscene AI 定位，prompt 可复用 |

**关键点**：NEP 的 `ai?.action` prompt 中的 `【】`、`[]` 括号风格可以在 Midscene 中改为 `''` 单引号风格，但内容语义保持一致即可。

### 规则 3：NEP `ai?.getElement(prompt)` + 后续原生操作 → Midscene 直接 AI 操作

```typescript
// NEP：先 AI 获取元素引用，再用原生方法操作
const budgetInput = await ai?.getElement('在 [Campaign budget] 下方的文本框');
await budgetInput?.scrollIntoView();
await budgetInput?.input('21');

// Midscene：一步到位
await agent.aiInput("21", "'Campaign budget'下方的输入框");
```

```typescript
// NEP：AI 获取元素判断存在性
const bidControl = await ai?.getElement('页面中的【Campaign budget】字样', { skipUndefined: true });
if (bidControl?.isExisting()) { break; }

// Midscene：用 aiWaitFor
await agent.aiWaitFor("展示'Campaign budget'元素");
```

### 规则 4：NEP scroll 循环找元素 → Midscene aiScroll 或 aiWaitFor

```typescript
// NEP：wheel 循环 + AI 判断元素存在
for (let i = 0; i < MaxRetry; i++) {
    await page.mouse.wheel({ deltaX: 0, deltaY: 1200 });
    const el = await ai?.getElement('目标元素', { skipUndefined: true });
    if (el?.isExisting()) break;
}

// Midscene 方案 A：单次 aiScroll（已知大致距离）
await agent.aiScroll('起始锚点', {scrollType: "singleAction", distance: 1200, direction: "down"});

// Midscene 方案 B：让 AI 持续滚动直到看到目标
await agent.aiAction("向下滚动页面直到看到 Campaign budget");
```

### 规则 5：NEP selector 兜底 AI 模式 → Midscene 直接 AI

```typescript
// NEP：selector 找行 → 找不到走 AI
const findElement = await this.findElementInRows(name);
if (!findElement) {
    let el = await this.ai?.getElement(`列表中的[${name}]文本`, { skipUndefined: true });
    await el?.hover();
} else {
    await findElement.hover();
}

// Midscene：直接 AI（不需要两条路径）
await agent.aiHover(`列表中的'${name}'文本`);
```

**关键点**：NEP 的 "selector 优先 + AI 兜底" 双路径模式，在 Midscene 中合并为单一 AI 路径。迁移时只需保留 AI 那条路的 prompt。

### 规则 6：BaseComponent 组件 → 提取 prompt 字段直接用

当 NEP case 中调用了封装过的组件（如 `listPage.editAdSubmitBtn.click()`），迁移方式：

1. 找到该组件的类定义（如 `EditAdSubmitBtn`）
2. 提取 `DEFAULT_PROMPT` 字段值（如 `'[Submit]按钮'`）
3. 直接作为 Midscene 的 prompt：`agent.aiTap("点击Submit按钮")`

如果组件没有 `prompt` 字段只有 `selector`，则需要在页面上确认该 selector 对应的 UI 元素，手写 prompt。

### 规则 7：等待策略的对应

| NEP 等待方式 | Midscene 等效 | 适用场景 |
|-------------|-------------|---------|
| `waitForPageLoadStable(page)` | `waitForPageLoadStable(page)` | **可直接复用**，midsence 项目已 import 此函数 |
| `page.wait(5000)` | `sleep(5000)` 或 `await new Promise(r => setTimeout(r, 5000))` | 硬等待（尽量用 aiWaitFor 替代） |
| `page.waitForResponse('/api/xxx')` | 无直接等效 | Midscene 如需接口校验需用 page 原生 API |
| `ai?.getElement(prompt, {skipUndefined: true})` 判断存在 | `agent.aiWaitFor("展示'xxx'元素")` | 等待某元素出现 |

### 规则 8：iframe 切换

```typescript
// NEP 中常有 iframe 切换
await page.switchToRootFrame();
// 组件中有 frameSwitch 配置：
static DEFAULT_EXT: ElementInfoExt = {
    frameSwitch: [{ type: "iframe", parentSelector: '[id="iframe"]' }]
}

// Midscene：如果页面有 iframe，需要在 agent 操作前手动切换
await page.switchToRootFrame();  // 可直接复用
// Midscene agent 默认在当前 frame context 中操作
```

---

## 四、NEP prompt 风格 → Midscene prompt 风格对照

| NEP prompt 风格 | Midscene 推荐风格 | 说明 |
|---------------|-----------------|------|
| `页面中的【Campaign budget】字样` | `展示'Campaign budget'元素` | 用于 waitFor/存在性判断 |
| `在 [Campaign budget] 下方的文本框` | `'Campaign budget'下方的输入框` | 用于定位输入框 |
| `点击页面左侧的[Campaign]文本` | `点击'Campaign'` | 用于 tap |
| `将鼠标hover在【xxx】上` | 无需翻译，直接用 aiHover | hover |
| `点击[xxx]下方带铅笔图标的[Edit]按钮` | `点击'xxx'下方带铅笔图标的Edit按钮` | 复合描述 |
| `列表中的[xxx]文本左边的开关` | `列表中'xxx'文本左边的开关` | 定位开关类元素 |
| `在Number of copies下面的输入框输入【1】` | 拆成 `agent.aiInput("1", "Number of copies下面的输入框")` | action 含输入拆成 aiInput |

**核心差异**：
- NEP 的 `ai?.action` 是一句话描述"做什么"（包含动作+目标），Midscene 把动作类型拆到了 API 方法名上（aiTap/aiInput/aiScroll/aiHover），prompt 只描述"在哪里"
- NEP 的方括号 `【】`/`[]` 在 Midscene 中统一改为单引号 `''` 即可

---

## 五、完整迁移 Checklist

迁移一个 NEP case 到 Midscene 时，按以下步骤执行：

1. **标准模板**：用 Midscene 标准 describe/it 结构，获取 `agent`
2. **逐行翻译**：遍历 NEP case 的每一行代码
   - 纯 selector → 查看 UI 对应元素 → 写 AI prompt
   - `ai?.action(prompt)` → 根据动作类型拆到 `agent.aiTap/aiInput/aiHover` + prompt
   - `ai?.getElement(prompt)` + 后续操作 → 合并为一个 `agent.aiXxx` 调用
   - selector 兜底 AI 双路径 → 只保留 AI 路径
3. **等待处理**：`waitForPageLoadStable` 可直接复用；`page.wait` 改 `aiWaitFor`
4. **iframe**：保留 `page.switchToRootFrame()` 调用
5. **before/after**：cookie 设置、mock intercept、清理逻辑照搬
6. **断言**：NEP 的接口校验（`waitForResponse`）需评估是否用 `agent.aiAssert` 替代或保留原生方式
