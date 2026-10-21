# 思考档位（reasoning effort）

> **档案状态**：🟢 形态已校准（A 级：档位枚举与每模型取值来自真实目录响应）；
> 行为部分为 🟡 B 级（代码实现，未逐条实测）
> **最后核对**：2026-10-22
> **影响级别**：L1（对话请求体正确性）

## 0. 结论

- 档位枚举（**实测**）：`low` / `medium` / `high` / `xhigh` / `max`，另有 `off`（可关思考）；
- `default_effort` **22 个模型全部下发**；`supported_efforts` 只有 14 个模型下发；
- 真实默认值不只 `high`：**实测出现 `medium`（9 个）、`high`（9 个）、`max`（2 个）三种**；
- 对 DeepSeek 系，"开思考"必须 **`thinking.type=enabled` + 带档位的 `reasoning_effort` 双写**，
  只写前者上游仍按不思考应答（代码注释记录的逆向结论，未单独复测）。

## 1. 档位排序（代码唯一事实源）

`internal/upstream/payload.go`：

```go
var effortRank = map[string]int{"off": 0, "minimal": 1, "low": 2, "medium": 3, "high": 4, "xhigh": 5, "max": 6}
```

`minimal` 在表里但**实测目录从未下发**——保留为兼容位，别据此假设上游会用。

## 2. 实测分布（2026-10-22，cn 面）

| default_effort | 模型数 | 例子 |
|---|---|---|
| `medium` | 9 | `glm-5.1`、`kimi-k3-1`、`minimax-m3`、`glm-5v-turbo`、`kimi-k2.6/2.7` |
| `high` | 9 | `glm-5.3`、`glm-5.2`、`deepseek-v4-flash/pro`、`deepseek-v4.1-flash`… |
| `max` | 2 | `space-bunny`、`minimax-m3.1-flash` |
| 未下发 | 2 | `auto`、`hy4-preview-f`（`efforts` 未下发但有默认档） |

`supported_efforts` 出现过的组合：`low/high/max`、`high/xhigh`、`low/high/xhigh`、
`low/medium/high`、`low/high`、`high`、`low/medium/high/xhigh/max`。

> **这张表的用法**：`default_effort` 缺省曾被硬编码为 `high`，但实测有 11 个模型默认不是 `high`
> ——所以**必须读目录**，不能假设。`lookupDefaultEffort` 查不到时才回落硬编码 `high`（并 warn）。

## 3. 请求侧注入规则（B 级，代码实现）

`injectThinking(obj, defaultEffort)`：

| 客户端请求体形态 | 网关行为 |
|---|---|
| 有 `thinking.type=enabled` 但缺档位 | **补默认档**（读目录 `default_effort`，查不到回 `high`） |
| 有 `thinking.type=disabled` | **照抄**，并删掉 `reasoning_effort`（snake + camel 双字段） |
| 无 `thinking` 字段 / `type` 为空 | 注入 `{type:"enabled"}` + 补默认档 |
| 客户端显式给了 `reasoning_effort` | **一律不覆盖、不在此处降级**（降级交给下一步） |

适用范围：模型名前缀匹配 deepseek 系（对齐官方 `thinkingFormat:"deepseek"`）；GLM / Kimi 走各自格式，
不受这段影响。

## 4. 档位降级管线（B 级，代码实现）

`normalizeReasoningEffort(obj, efforts)`，仅当**目录缓存里有该模型的 `supported_efforts`** 时生效：

| 情形 | 结果 |
|---|---|
| 请求档位在支持列表内 | 原样透传 |
| 请求档位不支持 | 改为 **≤ 请求档位的最高支持档**（降级） |
| 支持档全高于请求档 | 取**最低**支持档（偏离最小） |
| 未知模型 / 未知档位 / 未带字段 / 模型未缓存 | 一律透传（不自作主张） |

例子：请求 `xhigh`，模型只支持 `low/high/max` → 出站 `high`；请求 `low`，模型只支持 `high` → 出站 `high`。

**为什么必须有这条**：档位不合规会被上游按参数错误拒（或静默降级成不思考），
现象是"模型变笨了"这种最难归因的反馈。

## 5. `reasoning_content` 回填（B 级）

- 门控：`thinking.type == "enabled"` **或** 消息里已有推理痕迹（`thinkingEnabled || hasTrace`）；
- 作用：assistant 历史消息缺 `reasoning_content` 时补回，满足上游
  `requiresReasoningContentOnAssistantMessages` 要求；
- `deepseek + disabled + 无痕迹` → **零改动**（官方口径：不该补）。

## 6. 与 `UNVERIFIED.md` 的对应

| 编号 | 原断言 | 本次校准结果 |
|---|---|---|
| #6 | 「思考档位：CN 面取 `CODEBUDDY_FALLBACK_MODELS`，`deepseek-v4.1-flash` 国际版**只有 `['high']`**」 | **部分校准**：CN 面实测 `deepseek-v4.1-flash` = `low/high/max`（默认 `high`）；**国际版未测**（本机只有 cn 号）。CN 面档位确实全量下发且默认值多样，这部分成立 |

## 7. 失效表现

| 症状 | 先看 |
|---|---|
| 模型"变笨"、不回思维链 | `thinking.type` 是否被客户端置 disabled；`injectThinking` 是否命中该模型前缀 |
| 上游 400 参数错、提到 effort/reasoning | 档位降级链路：目录缓存是否为空（`efforts` 缓存由 `FetchModels` 填充，进程重启后需一次拉取） |
| 面板档位列与官方客户端不一致 | 目录缓存过期，重进模型页触发刷新 |
| `off` 档报错 | 该模型 `can_disable_thinking=false`，不能关思考 |
