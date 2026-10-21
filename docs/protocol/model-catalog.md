# 模型目录与能力字段

> **档案状态**：🟢 已校准（A 级：字段值与 22 个模型的档位全部来自真实响应）
> **最后核对**：2026-10-22 · **核对方式**：`cmd/doctor -models`（导出目录字段）+ `doctor` 的 `models` 探针
> **样本**：1 个 cn 个人号 · 22 个模型
> **影响级别**：L1（`/v1/models` 对外暴露的能力字段）+ L0（积分保底的收费判定兜底）

## 0. 结论

- 目录当前 **22 个模型**（cn 面），**全部** `supports_tool_call=true`、`supports_images=true`；
- 档位字段齐全：**14 个模型下发 `supported_efforts`**，**22 个全部下发 `default_effort`**；
- 促销体系确实在下发（`夜间折扣 0.50x` / `限时免费 0x` / `错峰使用` / `独家限免`），
  与 README「生效价 + 划线牌价」的说法一致；
- 一处**待复核的观察**：`glm-5v-turbo`、`kimi-k2.7`、`kimi-k2.6` 的 `credits` 解析出字面量
  `credits`（不是 `x0.xx` 形态）。可能是上游这几款的倍率字段形态不同（按次计费？），
  也可能是解析键位漂了——**不做结论，已登记为观察项**（见 §5）。

## 1. 端点

| 项 | 值 |
|---|---|
| 企业/个人目录 | `GET {chatBase}/console/enterprises/personal/models` |
| v3 目录 | `GET {chatBase}/v3/config`（含 `models` / `modelPromotions`） |
| 浏览器：网关侧 | `GET /v1/models`（对外，带能力字段）、面板「模型与档位」 |
| 代码 | `internal/upstream/model_catalog.go`、`global_models.go`、`modelsdev.go`、`context_catalog.go` |

`FetchModels` **并行打两路**（企业端点 + `/v3/config`）后合并；两路全失败才返回错误。

## 2. 四级查找链（缺字段时逐级回落）

```
① 上游目录（本次响应）
② 内置知识表（编译进代码的兜底）
③ data/model.json 缓存（上次成功拉取）
④ models.dev 公开目录
```

用途：`context_length` / `max_output_tokens` 这类字段上游偶尔不返回，链路保证面板与
`/v1/models` 不出现空洞。**改动这里要同步 `context_catalog.go` 的兜底表**。

## 3. 字段清单（上游键 → `ModelInfo`）

| 上游键 | 字段 | 说明 |
|---|---|---|
| `id` / `name` | `ID` / `Name` | 模型标识与展示名 |
| `maxInputTokens` | `ContextWindow` | 上下文窗口 |
| `maxOutputTokens` | `MaxTokens` | **输出上限（声称值）**——普遍虚高，实测钳制见 README「探测模型真实输出上限」 |
| `maxAllowedSize` | `MaxAllowedSize` | 与 `maxInputTokens` 并列下发，口径各自独立 |
| `reasoning.supportedEfforts[]` | `Efforts` | 思考档位；**空 = 未知/固定档** |
| `reasoning.defaultEffort` / `reasoning.effort` | `DefaultEffort` | 默认档（新模型用前者、老模型用后者） |
| `reasoning.canDisableThinking` | `CanDisableThinking` | 思考可关 → `off` 档可用 |
| `credits` | `Credits` | 积分倍率**牌价**原文（如 `x0.69`） |
| `modelPromotions` | `PromoFactor`/`PromoCredits`/`PromoLabel`/`PromoNote` | 当前生效优惠 |
| `supportsToolCall` / `supportsImages` / `supportsReasoning` | 同名 | 能力标记 |
| `onlyReasoning` | `OnlyReasoning` | 纯推理模型 |
| `tags` / `vendor` / `descriptionZh` | `Tags`/`Vendor`/`Description` | 标签、厂商、中文描述 |
| `isDefault` | `IsDefault` | 是否默认模型 |

## 4. 实测目录快照（2026-10-22，cn 面 22 个）

| ID | 上下文 | 输出上限 | 档位 | 默认档 | 牌价 | 促销 |
|---|---|---|---|---|---|---|
| `glm-5.3` | 1M | 131072 | low/high/max | high | x0.69 | — |
| `glm-5.2` | 1M | 131072 | high/xhigh | high | x0.79 | 夜间折扣 0.50x |
| `deepseek-v4-flash` | 1M | 50000 | low/high/xhigh | high | x0.17 | 错峰使用 |
| `auto` | 256K | 32000 | —（未下发） | high | —（未下发） | — |
| `hy4-preview` | 1M | 64000 | high | high | x0.29 | 夜间免费 0x |
| `deepseek-v4-pro` | 1M | 393216 | low/high/xhigh | high | x0.51 | 错峰使用 |
| `glm-5.3-flash` | 1M | 131072 | low/high/max | high | x0.08 | — |
| `glm-5.1` | 200K | 48000 | —（未下发） | medium | x0.79 | — |
| `kimi-k3-1` | 1M | **1048576** | —（未下发） | medium | x1.62 | — |
| `mimo-v2.6-pro` | 1M | 128000 | —（未下发） | high | x0.18 | — |
| `hy3-x` | 192K | 64000 | low/high | high | x0.05 | — |
| `step-5-preview` | 1M | 64000 | low/medium/high | medium | x0.43 | 独家限免 0x |
| `kimi-k2.8-preview` | 1M | 131072 | low/medium/high | high | x0.77 | — |
| `hy3` | 192K | 64000 | low/high | high | x0.00 | 限时免费 0x |
| `space-bunny` | 1M | 128000 | low/medium/high/xhigh/max | **max** | x0.08 | — |
| `minimax-m3.1-flash` | 1M | 128000 | low/medium/high/xhigh/max | **max** | x0.08 | — |
| `hy4-preview-f` | 1M | 64000 | high | high | —（未下发） | — |
| `deepseek-v4.1-flash` | 1M | 393216 | low/high/max | high | x0.11 | — |
| `minimax-m3` | 512K | 524288 | —（未下发） | medium | x0.25 | — |
| `glm-5v-turbo` | 200K | 64000 | —（未下发） | medium | `credits`⚠ | — |
| `kimi-k2.7` | 256K | 32000 | —（未下发） | medium | `credits`⚠ | — |
| `kimi-k2.6` | 256K | 32000 | —（未下发） | medium | `credits`⚠ | — |

> 原始导出（含 vendor / tags / promo_note）见 `cmd/doctor -models` 的输出。
> 这张表的价值在于**可对比**：下次跑一遍，哪个模型消失了、档位变了、价格变了，一眼可见。

## 5. 观察项（待复核，不急）

1. **`credits` 字面量异常**：`glm-5v-turbo` / `kimi-k2.7` / `kimi-k2.6` 的倍率是 `credits` 而不是 `x0.xx`。
   影响：面板「模型与档位」的价格列会显示 `credits`；`pool.credit_floor` 的收费判定依赖倍率表，
   解析不出数字的模型会走 tier 1（无观测）分支——**不影响正确性，但会让保底判据退化**。
   复核方式：下次跑 `doctor -models` 看这三款是否仍是该形态；若一直如此，扩展解析即可。
2. **`kimi-k3-1` 声称输出 1M（1048576）**：与 README「声称值普遍虚高、多个模型被静默钳制」一致，
   真实上限需 `probe_max_tokens.py` 实测（**花积分**，未跑）。
3. **档位未下发的 9 个模型**：`efforts` 为空表示"未知/固定档"，网关据此走「不降级、原样透传」分支
   ——不是"没有思考能力"。

## 6. 判据与使用边界

- **`/v1/models` 只透出、不参与路由**：模型是否可用由账号池按域/模型冷却判定（`model_locks`）。
- **唯一参与决策的是倍率**：`pool.credit_floor` 在本地成本账本无观测时，用目录倍率判断
  「这个模型是否收费」，从而决定触底账号能否承接该模型请求。解析不出倍率 = 退化到"无观测"（放行），
  这是已知的兜底弱点（README 有实案说明：kimi-k3-1 曾因此打穿两个账号）。
- **档位降级**见 `thinking-effort.md`。

## 7. 失效表现

| 症状 | 先看 |
|---|---|
| `/v1/models` 空列表 | `model_catalog.go` 两路是否都失败（日志有 `fetch models: ... failed`） |
| 模型在列表里但 chat 报 11102 | 不是目录问题——该账号对此模型无权限，属模型级负缓存（`model_locks`） |
| 面板最大输出列显示值与实际不符 | 声称值 vs 实测值，跑 `probe_max_tokens.py` |
| 价格列显示 `credits` 等怪值 | 见 §5.1 观察项 |
