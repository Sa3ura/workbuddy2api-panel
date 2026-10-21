# wire 层契约（SSE 帧 / DSML 标记 / tool 配对）

> **档案状态**：🟢 已锁定（**A 级**：每条规则都有对应测试断言，测试名即证据）
> **最后核对**：2026-10-22 · **核对方式**：`go test ./internal/upstream/ -run 'Aggregate|Stream|Normalize|PrepareBody|MarkupRepair|Tool|Truncat'`
> **影响级别**：L1（"能连但跑不通"这一类最难排查的问题全在这一层）

## 0. 这一层为什么必须写下来

网关对外是 OpenAI 兼容协议，对内是 DeepSeek 系的私有形态。两者之间的**全部翻译**都在这里：
SSE 帧要重建、工具调用标记要还原、会话历史要整形。任何一处改错，症状都是
"客户端能连上、也能出字，但工具不执行/会话卡死"，**不在 HTTP 状态码里体现**。

好处是：这一层的语义已被 90+ 个测试锁死（`sse_test.go` 49KB / `dsml_test.go` 21KB /
`tool_pairing_test.go` 22KB）。**改这一层之前先跑测试，测试名字就是需求文档。**

## 1. 出站请求体管线（`payload.go`，顺序不可换）

```
prepareBody
 ├─ 强制 stream:true                     （无论客户端要流式与否）
 ├─ translateMaxCompletionTokens          （OpenAI 别名 → 上游 max_tokens；别名一律删）
 ├─ tool_choice 归一                      （function 对象 / none / auto 三态）
 ├─ image_url 字符串 → 对象形态
 ├─ injectThinking(obj, defaultEffort)    （见 thinking-effort.md）
 ├─ normalizeReasoningEffort(obj, efforts)（≤请求档的最高支持档）
 ├─ backfillReasoningContent(obj)         （多轮一致性）
 ├─ resolveToolPairing / 孤儿 tool_call 清理（见 §4）
 └─ sanitizeMessages（指纹脱敏，可关）
```

测试锚点：`TestPrepareBodyForcesStream`、`TestPrepareBodyToolChoice*`、
`TestPrepareBodyMergesSplitParallelToolCalls`（管线顺序影响结果，改序会红）。

## 2. SSE 帧重建（`sse.go`）

### 2.1 白名单重建（`normalizeFrame`）

**只保留 OpenAI 流式规范字段**，上游噪声一律剔除：

| 上游形态 | 网关输出 |
|---|---|
| `finish_reason: ""` | `null`（空串不是合法值） |
| 空 `content` / 空 `refusal` | 删字段 |
| 空 `tool_calls` 列表 | 删字段 |
| 其它未知字段 | 剥离 |

`reasoning_content` **保留**（DeepSeek 思维链靠它透出）。

### 2.2 tool_calls 分片合并

上游把一次工具调用拆成多帧，网关按 `index` 合并：

- **首帧保留 `function.name`**，同一 `index` 的后续分片**删掉 `name` 键**（无论上游是空串还是重复）；
- 后续分片 `name` 键**缺失**（而不是空串）时，保留已建好的首帧 name——缺失键比空串安全
  （`??` 与 truthy 守卫对缺失必然保留旧值）；
- 并行工具调用（多 `index` 交错）各自独立合并。

测试锚点：`TestStreamToolCallNameOnce`、`TestStreamToolCallParallelFragments`、
`TestStreamToolCallNoiseEmptyName`、`TestAggregateToolCalls`。

### 2.3 收尾保证

| 规则 | 说明 | 测试 |
|---|---|---|
| **恰好一个 `data: [DONE]`** | 上游漏发时兜底补写 | `TestStreamDoneFallback` |
| `[DONE]` 之后的垃圾帧丢弃 | 不当成新内容 | `TestStreamGarbageAfterDone` |
| 空流先写一帧 `error` 再补 `[DONE]` | 避免客户端永久等待 | `TestAggregateEmptyStreamError`、`TestStreamEmptyFramesCase` |
| 首帧 `id` 透传；无 id → 哨兵值 | 客户端据此关联 | `TestStreamFirstIdPassthrough`、`TestStreamNoIdFallsBackToSentinel` |
| 非 data 行（注释/心跳）跳过 | — | `TestAggregateSkipsNonDataLines` |

### 2.4 非流式聚合

`Aggregate` 把整条 SSE 流合成单个 `chat.completion`（含 `reasoning_content` / `tool_calls`），
空流按 §2.3 的 error 帧语义处理。

`usage` 缓存别名归一：`normalizeUsageCacheAliases` 把嵌套命中数镜像到顶层，**零值也保留**
（`TestNormalizeUsageCacheAliasesPreservesZeroResult`——把 0 当"没有"会算错费用）。

## 3. DSML 原生工具调用标记（`dsml.go`）

### 3.1 上游语法

模型的原生工具调用**不是** OpenAI 的 JSON `tool_calls`，而是一段带**全角竖线**（U+FF5C）的标记文本：

```text
<PIPE>DSML<PIPE> calls>
<PIPE>DSML<PIPE> invoke name="exec_command">
<PIPE>DSML<PIPE> parameter name="cmd" string="true">ls -la<PIPE>DSML<PIPE> parameter>
</PIPE>DSML<PIPE> invoke>
</PIPE>DSML<PIPE> calls>
```

（`<PIPE>` 是占位，真实分隔符是 `markupPipe = "\uFF5C\uFF5C"` 两个全角竖线。）

**上游只在请求声明了 `tools` 时才把它解析成结构化 `tool_calls`**。而 `tools` 在链路里丢失是常态
（协议转换丢字段、一次性 exec 探测、auto-review 复核回合），此时标记以纯文本落进 `content`，
只认结构化 `function_call` 的客户端（如 Codex）会把它当正文写进历史，**回合直接结束、工具一次没跑**。

### 3.2 判定的两级 + 三条硬前提

| 工具名单状态 | 判定 |
|---|---|
| 非空（严格） | 块内**每个** invoke 的工具名都必须在名单里 |
| 为空（弱，`tools` 丢失的常态） | 块必须**完美闭合** + 块内不夹带正文 + 工具名形如合法标识符（`markupToolNameRe`） |

工具名单有两个来源，二者会分别缺失，**都要**：本请求声明的 `tools[].function.name` /
`tools[].name` / `functions[].name`，以及会话历史 `messages[].tool_calls[].function.name`。

三条硬前提（任一不满足即**原文逐字节透出**）：

1. 本回合上游**没有**给出任何结构化 `tool_calls`（绝不覆盖真实调用）；
2. 块内除 invoke / parameter 与空白外**不夹带正文**（模型"讨论"这段语法时不该被还原）；
3. 非流式额外要求 `finish_reason == stop`（`length` 表示正文被截断，不作解释）。

> 弱判定的存在理由：`tools` 丢了才最需要还原，此时名单为空。用"完美闭合 + 名字形态"拦住
> 模型在正文里举例写出的伪标记（`TestMarkupRepairWeakModeNameShape`、
> `TestMarkupRepairWeakModeStillRejectsMalformed`）。

### 3.3 逐字节等价保证

- 未闭合块、参数畸形、超长（>256KB）→ **一律回吐原文**，绝不吞字节；
- `<` `>` 普通尖括号不受影响（只认完整 `<||DSML||` 前缀）；
- 任意 chunk 边界切开都能还原（流式跨块拼接，`TestMarkupRepairAnyChunkBoundary`）；
- 有 `TestMarkupRepairByteEquivalence` 做逐字节回归。

排障日志（区分"没识别到"与"识别到但判定不通过"）：

```text
INFO: [server] stream …: repaired N native tool call(s) from assistant text (tool names known=K)
WARN: [server] stream …: saw M native tool call block(s) but repaired none (tool names known=K)
```

## 4. tool 配对与会话自愈（`tool_pairing.go`）

### 4.1 为什么必须整形

OpenAI 兼容协议要求：带 `tool_calls` 的 assistant 消息，**每个** `tool_call.id` 必须有对应的
一条 `role:tool` 结果消息；反之 `role:tool` 也必须有前置 `tool_call`。**缺任一侧，上游直接
HTTP 400 拒绝整个请求。**

工具执行失败时（参数非法、超时、工具不存在），客户端会把 assistant 的 `tool_calls` 持久化进
会话历史，却写不回结果消息。这条坏历史随后被每次请求原样重放 → 上游对之后**每一条**用户消息
都返回 400 → 整条会话报废。

网关是最后一道防线：**发请求前删掉无法配对的条目让会话自愈**——宁可丢一轮工具上下文，
也好过整条会话死亡。

### 4.2 四种整形动作

| 动作 | 语义 | 测试 |
|---|---|---|
| 清理孤儿 `tool_call` | assistant 有 `tool_call` 但缺少对应 `role:tool` → 删该调用（全删则删整条消息） | `TestCleanupOrphanToolCalls` |
| 清理孤儿 `tool` 结果 | `role:tool` 找不到前置调用 → 删该消息 | 同上 |
| 合并相邻 `tool_calls` | 并行调用被拆成多条 assistant 消息时合并 | `TestMergeAdjacentToolCalls`、`TestPrepareBodyMergesSplitParallelToolCalls` |
| 结果块重排 | `role:tool` 顺序与 `tool_calls` 声明顺序对齐 | `TestRepackToolResultBlocks` |

边界：空名工具调用也纳入清理（`TestCleanupOrphanToolCallsEmptyName`）；
`content: []` 的空数组形态也要能合（`TestMergeAdjacentToolCallsEmptyArrayContent`）。

## 5. 半截工具参数（`truncation.go`）

SSE 被截断（连接中断 / `finish_reason==length`）时，`arguments` 可能只剩半截 JSON。
原样下发 → 客户端解析报错并卡死会话。

**处置：丢弃该次残缺调用并触发重试（报 max-tokens），绝不补 `{}` 伪装合法。**

关键区分（`isTruncatedArguments`）：

| 形态 | 判定 |
|---|---|
| **非空但无法解析** | 视为截断 → 丢弃 |
| 空串 | **合法**（无参数工具），保留 |
| 能解析但类型不对（标量/数组） | 模型输出错误，交给客户端 schema 校验回传，**不在网关判定** |

此前的实现只认 `finish_reason=="length"`，EOF 截断的残缺参数被原样下发——已修
（`truncation.go` 头部注释记录了这次修复）。

## 6. 失效表现（排障对照）

| 症状 | 先看 | 关键测试 |
|---|---|---|
| 工具不执行、`content` 里出现 `<\|DSML\|` 文本 | `dsml.go` 判定：名单是否为空、块是否闭合、有无夹带正文 | `MarkupRepair*` |
| 客户端报"非 JSON 参数"并卡死 | `truncation.go`：是否漏判了非 `length` 的截断 | `Truncat*` |
| 上游 400 且提到 tool/tool_calls 配对 | `tool_pairing.go` 四种整形是否都生效 | `CleanupOrphan*` / `Repack*` |
| 工具参数缺字段 / name 被清空 | `sse.go` 的按 index 合帧 + name 保留规则 | `StreamToolCallNameOnce` |
| 流不结束 / 客户端一直转圈 | 是否恰好一个 `[DONE]`；空流是否补了 error 帧 | `StreamDoneFallback` |
| `finish_reason` 是空串 | 白名单重建是否生效 | `TestNormalizeFrame` |

## 7. 改动纪律（这一层的三条铁律）

1. **改前跑测试**：`go test ./internal/upstream/`。测试名就是需求，红了先读测试再改代码。
2. **绝不吞字节**：修复层要么整体还原，要么原文透出；不存在"删掉一半"的中间态
   （`TestMarkupRepairByteEquivalence` / `TestMarkupRepairUnclosedFlushedVerbatim` 守着）。
3. **不覆盖真实调用**：本回合有结构化 `tool_calls` 时，正文里的标记一律当正文
   （`TestMarkupRepairProseInsideBlockStaysText`）。
