# 待验证事实清单（UNVERIFIED）

> 本文件登记**目前被代码/注释/README 断言、但没有可复现证据**的上游行为。
> 它是 P3 `doctor` 探针的**靶子**：每校准一条，就把状态改成 ✅ 并把证据写进对应协议档案。
>
> 背景：原始逆向文档已确认不可得（见本目录 `README.md` §0），这些断言目前只有 **B 级**可信度——
> 可能来自过去某次真实抓包，但上游随时可能已改。**不要把这里的任何一条当成事实去做决策。**

级别：**A** = 本机真实响应实测 / **B** = 代码反推或转引旧实测（未复核）

| # | 断言 | 出处 | 影响 | 校准探针 | 状态 |
|---|---|---|---|---|---|
| 1 | 桌面端任务靠特定事件链点亮（`workbuddy-desktop` 事件族 + `WorkBuddy/5.5.6` UA），缺一环 200 不计分 | `upstream/desktop.go:3,300` | L3 | `doctor:tasks`（任务码集合）+ 单号小样验证 | ⬜ B |
| 2 | `/v2/report` 缺 `userId` 时返回 200 但**静默丢弃**，progress 不动 | `upstream/report.go:4`、`scheduler/scheduler.go:579` | L2 | `doctor:streak` 回读天数 | ⬜ B |
| 3 | 小程序口径任务需 `X-Client-Platform: miniprogram`；`school_season` 无 `activityId` 不点亮 | `upstream/school.go:3`（原 `protocol.md §7.11`） | L3 | `doctor:tasks`（mp 头下的任务码集合） | ⬜ B |
| 4 | 企业号个人成长端点一律拒绝：签到 `400 code 10001`、growth `403` | README「企业版账号」节（2026-10-07 A/B 对照） | L2 | `doctor:*` 对企业号应返回 403/10001（**403 即绿**） | ⬜ B |
| 5 | `429 code 6004` 为**模型级**限流，msg 带"将在 … 重置"时冷却到该墙钟 | `upstream/client.go:127,147` | L0 | 触发时抓 msg 解析（被动观测，不主动造） | ⬜ B |
| 6 | 思考档位：CN 面取 `CODEBUDDY_FALLBACK_MODELS`，`deepseek-v4.1-flash` 国际版**只有 `['high']`** | `upstream/effort_catalog.go:8,9,24,45` | L1 | `doctor -models` | 🟡 **部分校准（2026-10-22）**：CN 面实测 `deepseek-v4.1-flash` = `low/high/max`、默认 `high`；22 模型全部下发 default_effort（medium×9 / high×9 / max×2 / 未下发×2）。**国际版未测**（本机仅 cn 号）。详见 `thinking-effort.md` |
| 7 | 上游支持 `prompt_cache_key`，同一 8k 前缀命中后费用降约 17× | `upstream/cache_key.go:3` | L2（费用） | 被动观测 `prompt_cache_hit_tokens` | ⬜ B |
| 8 | 上游 `max_output_tokens` 声称值普遍虚高，多个模型被静默钳制 | README「探测模型真实输出上限」 | L1 | `scripts/probe_max_tokens.py`（**花积分**） | ⬜ B |
| 9 | 补偿接口 `active=false` 是"当前无活动"的正常 200，不是错误 | `upstream/compensation.go` | L2 | `doctor:compensation` | ⬜ B |
| 10 | `/agenttool/v1/search` 结果条数固定 5，`allowed_domains`/`freshness`/`blocked_domains` 静默失效 | README「联网搜索」节 | L2 | `doctor:search` | ⬜ B |
| 11 | WAF 403 拦的是**网关出口 IP** 而非账号（换号无效），需换出口 | `server/wafip.go:3`（原 `fork-scan-absorb T-1`） | L0 | 观测型：连续 403 时比对 IP/账号维度 | ⬜ B |
| 12 | global 新号需先完成注册地区（`login/register/user/complete`）才可激活 trial，否则 chat 报 `14017` | `upstream/global_register.go:3` | L2 | `doctor:account` + 一次 trial 领取 | ⬜ B |
| 13 | 错误码语义：`11101`/`11102`/`11158`/`12153`/`14017`/`14051`/`10001` | `upstream/client.go`、`hint.go`、README 错误分类表 | L0/L1 | 被动观测 + `doctor` 分类断言 | ⬜ B |
| 14 | 流是被截断时 `tool_calls.arguments` 为半截 JSON，须丢弃而非补 `{}` | `upstream/truncation.go` | L1 | 已有单测覆盖语义；上游行为属推测 | ⬜ B |
| 15 | `glm-5v-turbo` / `kimi-k2.7` / `kimi-k2.6` 的 `credits` 解析为字面量 `credits`（不是 `x0.xx`） | 2026-10-22 `doctor -models` 实测 | L1（保底倍率判据退化） | 下次 `-models` 复核是否持续 | 🟡 观察中（持续则扩展解析，见 `model-catalog.md` §5.1） |

## 校准规则

1. **探针只读**：校准过程不得签到 / 领奖 / 上报 / 建会话（见 P3 三条硬规则）。
2. **一条一证**：每条断言校准后，必须在对应协议档案写"日期 + 探针名 + 观察到的原文"。
3. **推翻要留痕**：若探针证明断言已失效，**不要直接删**——标 🔴 并写"失效于 <日期>，现行为 X"。
   这条比校准更有价值：它是上游变更的时间线证据。
