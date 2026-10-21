# 维护方案（接管版）

> 适用前提：原仓库 2026-10-21 声明停止维护。本文件给接手者一套**可执行**的维护策略，而不是"继续跟进上游"的承诺。
> 结论先行：**这个项目的维护重点不是写新功能，而是三件事——保住知识资产、把"上游协议漂移"从被动踩坑变成主动告警、把每次修复的成本压到最低。**

## 0. 作者停更的三条理由 → 三条维护约束

| 作者给出的理由 | 对维护的真实含义 | 本方案对策 |
|---|---|---|
| 上游开始禁反代（通道校验、账号限制），稳定性不可控 | **通道会衰减，且不由你决定**。不能假设"修好就长期可用" | 目标分级（§1）+ 风控降档预案（§6）+ 退出判据（§7）：把"部分功能失效"和"整体不可用"分开处理 |
| 个人精力有限，无力长期跟进协议变化与风控对抗 | 维护必须**低边际成本**：每次上游变更只允许付出"定位 + 改一处 + 跑 golden 测试" | 知识资产入库（§2.2）+ 症状索引（§2.4）+ 漂移检测（§3）+ 脆弱面地图（§4） |
| 非法转卖泛滥 | 公共维护的收益被滥用抵消；**不要为规模化/分发做优化** | 治理边界（§9）：只维护自用范围内的正确性，拒绝为批量分发服务的功能 |

配套原则：**只修不加**。接手期不引入新功能，除非它是"降低维护成本"的工具（探针、测试、文档）。

## 1. 维护目标分级（明确什么可以死）

| 级别 | 内容 | 失效意味着 | 维护投入 |
|---|---|---|---|
| **L0 必须活** | `/v1/chat/completions`（流式/非流式）、token 自动刷新、账号池选号与故障换号 | 项目失去存在意义 → 触发退出评估（§7） | 最高，随时修 |
| **L1 必须正确** | `/v1/models` 与模型能力字段、SSE 帧重建、`reasoning_content` 回填、DSML 工具调用还原、`tool_pairing` 请求体整形 | 客户端"能连但跑不通"（最难排查） | 高，随 L0 一起修 |
| **L2 尽量活** | `/v1/search/messages`、签到、token 保活、余额刷新与解冻、补偿查询 | 影响积分与搜索，不影响对话 | 中，按周修 |
| **L3 可以死** | 成长任务一键完成 17/18、开学季 5 任务、桌面事件链、夜猫子、猫猫旅行、连登管家 | **只影响薅积分，不影响网关可用** | 低，坏了先关开关，集中一批再修 |

> 这条分级是整套方案的支点：L3 的任何失效都**不该挡住一次 release**，L2 的失效可以用 `schedule.*_enabled: false` 关掉止损，只有 L0/L1 才值得熬夜。

## 2. 第一周：补齐"可维护性"（按顺序做）

### 2.1 固化构建基线

当前这台机器**没有安装 Go**，仓库也无法自证可构建——先解决这个。

```powershell
# 装 Go 1.27.3（与 go.mod 一致）；或用 Docker 构建
go build ./... ; go vet ./... ; go test ./...
```

- 记录一条 **green commit**（全绿的那个 hash）并打 tag（如 `v1.13.0-maintain-base`）。以后任何改动都以"回到 green commit"为退路。
- **版本对齐**（已解决 2026-10-22）：原先 `go.mod` 是 `1.22.5` 而 `Dockerfile` 用 `golang:1.23-alpine`。
  现已统一到 **1.27.3**（`go.mod` / `Dockerfile` / README / CI 四处同步），并顺带清掉 43 条标准库漏洞
  ——**教训是"版本漂移"不只是洁癖，它会直接变成 `govulncheck` 的恒红告警**。
- 构建脚本化：加 `Makefile` 或 `scripts/build.ps1` + `scripts/build.sh`，把 README「方式三」里散落的 4 条 `go build` 收成一条 `make all`。
- 依赖面很小（`go-redis`、`xxhash`、`rendezvous`、`atomic`、`x/sys`，且均为 indirect），供应链风险低，保持这个规模。

### 2.2 抢救知识资产（**最重要的一步**）

这是本项目最大的单点风险：**代码里 30+ 处注释在引用不存在于仓库的文档**——

```
ctx.md(8) · buddy-adapter.ts(3) · data/desktop-task-protocol.md(3) · REPORT-active-map.md(2)
buddy.ts(2) · sse.ts(2) · product.ts(2) · ANALYSIS-workbuddy-client-reverse.md(1)
ANALYSIS-global-chat-solutions.md(1) · transport-hardening.md(1) · fork-scan-absorb.md(1) …
```

而这些文件**进不了 git**：`.gitignore` 里有 `docs/`、`*.md`（仅 `!README.md` 例外）。也就是说：作者停更后，**"这段代码为什么这么写"的原始证据链只存在于某台机器上**。一旦丢失，任何一次上游变更都得从零逆向。

必须做：

1. 解除禁令（注意 git 不会进入被整体忽略的目录，必须先删 `docs/` 那条）：

   ```gitignore
   # 删掉：docs/
   # 改掉：*.md 与 !README.md 的组合 →
   !docs/**/*.md
   ```

2. 把本地已有的逆向文档（`data/desktop-task-protocol.md`、`ANALYSIS-*.md` 等）**脱敏后**搬进 `docs/protocol/`：删掉所有 token / uid / 手机号 / 昵称 / cookie，只留协议事实。**入库前先跑一遍 grep 查 `access|refresh|token|phone|Bearer` 字样。**
3. 每份档案顶部统一写四段：**端点 / 判据 / 实测日期 / 失效表现**（见 2.3）。
4. 承认差距：拿不到的文档，就在代码注释里把引用改成一段 3 行摘要（"原文档已不可得，此处依据 2026-09 实测：…"），把知识从注释里的**指针**变成**内容**。

### 2.3 协议档案模板（`docs/protocol/<主题>.md`）

```markdown
## <端点相对路径>   <方法>
- 域：chatBase / billingBase / webBase（见 internal/upstream/client.go:808）
- 必需头：UA 族 / X-Client-Platform / Origin / Referer / X-Device-Token / X-User-Id
- 请求体：<字段 + 必填性>
- 成功判据：HTTP 200 且 body.code == 0（**200 ≠ 生效**，见下）
- 幂等性：按天/按次/无；重复调用后果
- 实测日期 + 账号类型（个人 / 企业 / global）
- 失效表现：症状 → 日志特征 → 对应代码文件
```

「200 ≠ 生效」这条已经在 README 里被反复强调（计分异步落定、登记回读验证、`200 OK 但静默丢弃`），**这是本项目最容易误判的一类问题**，每份档案都必须写清"怎么确认真的生效了"。

### 2.4 症状 → 文件的三分钟索引

排障时最贵的不是修，是**定位**。把下面这张表贴在维护笔记首页（并随手补行）：

| 症状 | 先看 |
|---|---|
| 对话 400 `blocked by security policy` / `unapproved channel` | `upstream/sanitize.go`、`upstream/payload.go`、`prompt/prompt.go`（降级重试） |
| 对话 400 `Unmarshal chat params failed` / `11101` | `upstream/payload.go`、`upstream/tool_pairing.go`（请求体整形，不罚号） |
| 工具不执行 / 标记文本进正文 | `upstream/dsml.go`（还原判定）、`upstream/tool_pairing.go` |
| 流式卡住 / 重复帧 / 缺 `[DONE]` | `upstream/sse.go`（白名单重建）、`upstream/idle.go` |
| `reasoning_content` 丢失或回填报错 | `upstream/thinking.go` |
| 429 `code 6004` 冷却异常 | `upstream/client.go:127/147`、`pool/pool.go`（模型级冷却） |
| 签到 200 但没加积分 | `upstream/client.go`（签到）、`IsAlreadyCheckin` 判据、`pool` 余额回写 |
| 任务上报 200 但进度不动 | `upstream/tasks.go`、`upstream/desktop.go`（指纹错）、`upstream/school.go`（mp 口径） |
| 所有模型 404 / 模型列表空 | `upstream/model_catalog.go`、`upstream/global_models.go`、`upstream/modelsdev.go` |
| 全池 503 | `pool/pool.go` 选号过滤链（disabled/暂停/冷却/熔断/在途/积分保底） |
| UA 归因列变了 / 被判定非官方客户端 | `upstream/headers.go`、`upstream/device_token.go` |

### 2.5 建 CHANGELOG

仓库目前没有。用 `Keep a Changelog` 格式，每条至少写：**改了什么上游行为、依据哪份协议档案、影响哪一级（L0-L3）**。这是三个月后你唯一能信任的记忆。

## 3. 第二、三周：漂移检测（**性价比最高的新增能力**）

现状是：上游悄悄改协议 → 直到某天用户报错 → 你才发现 → 现场逆向。**把这套流程反转过来**：

### 3.1 `cmd/doctor`：只读探针矩阵

一个独立命令（也可做成 `/panel/api/upstream_health`），对每个账号跑一遍**只读端点**并校验**响应形状**，输出绿/黄/红：

| 探针 | 端点 | 免费 | 判据 |
|---|---|---|---|
| token 刷新可用性 | `plugin/auth/token/refresh` | 是 | 200 且返回新 accessToken |
| 模型目录 | `console/enterprises/personal/models` | 是 | 非空且含预期字段名集合 |
| 余额 | `billing/meter/get-user-resource` | 是 | 200 且结构可解析（**字段名变化即黄**） |
| 连登 | `activity/growth/streak` | 是 | 200（企业号应 403，作对照） |
| 任务列表 | `activity/growth/tasks` | 是 | 任务码集合与本地已知集合的交集比例 |
| 补偿状态 | `billing/meter/compensation-status` | 是 | 200 且 `active` 字段存在 |
| 猫档案 | `activity/growth/buddy/info` | 是 | 200 |
| 会话/权限 | `console/account`（`profile.go`） | 是 | 200 |
| 搜索 | `/agenttool/v1/search` | 是 | 返回 `web_search_result` 结构 |
| 对话（可选，需开关） | `chat/completions`，`max_tokens=8` | **否，花积分** | 有首帧且 `finish_reason` 正常 |

判级：**红** = HTTP 非 2xx 或 code≠0；**黄** = 2xx 但字段缺失/新增/任务码交集下降（**形状漂移**，是"要变天了"的最早信号）；**绿** = 符合预期。

三条硬规则：
- **不写操作**：不签到、不上报、不领奖、不建会话——探针必须可随时随地被任何人跑，不产生副作用与积分消耗。
- **形状校验优先于状态码**：上游改字段名时会返回 200，这是最阴的一类变更。
- 结果落 `data/upstream_health.json`（同 `output_probes.json` 的模式，面板只读透传），**复用既有 `model_probes` 通路即可，不用动前端框架**。

### 3.2 排程与告警

挂在现有调度器里（例如每小时或每日一次），**黄/红状态写 warn 日志行并进面板「运行日志 → 任务」频道**——项目已有"日志分频道 + 面板卡片"的基础设施，不需要新建通知系统。真要外发，用退出码 + 宿主 cron 即可。

> 价值：把"上游协议变化"从**事故**降级为**告警**。有了它，你可以合理地"不盯"，而不是"不敢睡"。

## 4. 脆弱面地图（`internal/upstream` 16.5k 行的分级）

| 耦合度 | 文件 | 编码的上游知识 | 变了会怎样 |
|---|---|---|---|
| **极高**（随时会变，坏了功能哑掉） | `headers.go`(19K) | UA 三段式、`X-IDE-*` 族、Origin/Referer、`X-Client-Platform` | 判定非官方客户端 → 全量 403/风控 |
| 极高 | `device_token.go` | 设备风控头三源 | 同上 |
| 极高 | `desktop.go`(25K) | 桌面事件链（RichMeow / Buddy_App / 主题 / 模板…） | L3 任务全部哑火 |
| 极高 | `school.go` / `tasks.go` / `streak.go` / `travel.go` / `blackcat.go` / `report.go` / `compensation.go` | 任务判据、领奖域、连登档位、旅行状态机 | L3/L2 功能静默失败（最坑） |
| 高 | `search.go` | `/agenttool/v1/search` 出入参 | L2 搜索失效 |
| 中（结构变了可以本地修） | `sse.go`(33K) / `dsml.go`(18K) / `payload.go`(19K) / `thinking.go` / `tool_pairing.go` / `truncation.go` / `idle.go` / `sanitize.go` / `hint.go` | wire 形态、标记语法、错误码语义（11101/11133/11135/12153/14051…） | 对话"能连不能跑"，排查最耗时 |
| 中低 | `model_catalog.go` / `global_models.go`(26K) / `modelsdev.go` / `context_catalog.go` / `effort_catalog.go` | 模型目录与能力聚合 | 列表/能力显示错，不影响对话 |
| 低（本地逻辑，随 CI 保障） | `pool/*`(7.7K) / `panel/*`(7.9K) / `reqlog` / `usage` / `credithist` / `httpauth` / `livecfg` | 调度策略、持久化、面板 | 自己写坏的，测试能兜住 |

推论：**维护预算的 80% 会花在表格前 8 行**。所以 `headers.go` / `desktop.go` / `tasks.go` 必须优先获得 golden 测试（当前 `sse/dsml/payload/useragent` 已有较强测试，桌面与任务链偏弱）。

已有的**可配置逃生门**要用足：`upstream.client_version` / `cli_version` / `user_agent` 都已可覆盖版本号（`headers.go:19-22`），上游一改版本就能改配置而不是改代码——这是本项目最聪明的设计之一，继续延用这个模式。

## 5. 工程加固（持续，低强度）

| 项 | 动作 | 说明 |
|---|---|---|
| CI | 现有 `go-binaries.yml` 已做 `go test ./...` + 五平台构建 + 版本串断言 | 保留，别动发布纪律 |
| 静态检查 | 加 `go vet ./...`、`gofmt -l .`、`staticcheck` | 成本近零，能抓 `pool` 并发疏漏 |
| 竞态 | 给 `pool` 与 `upstream` 加一轮 `go test -race ./internal/pool/...` | 池子有多处锁与在途计数 |
| 依赖 | Dependabot（go 模块） | 依赖只有 5 个，升级摩擦极小 |
| 漏洞 | 季度跑 `govulncheck ./...` | 主要是 Redis 客户端与 `x/sys` |
| 测试 | 把 golden 快照扩到 `desktop.go` / `tasks.go` 的请求体 | 与 §3 的"形状校验"共用样本 |
| 文档 | README 的「关键断言 ↔ 代码出处」表**已带行号**——行号会随改动失效，改为"文件 + 符号名"更耐久 | 现在就开始，否则半年后全是错行 |

## 6. 风控降档预案（应对"上游开始禁反代"）

先把**信号**定义清楚（这些都能进 §3 的探针）：

- 出现 HTML 响应 / Cloudflare 挑战 / 验证码 → WAF 层拦截
- 文案类：`illegal api invocation`、`unapproved channel`、`invalid client`、`device` 相关 403
- 全池同时 401/403 且刷新 token 无效 → 通道级封禁
- 单账号短时间内大面积 429 而积分充足 → 行为风控
- 同一 IP 多号同时受限 → IP 维度策略

**现成的降档旋钮**（全在配置里，不需要改代码）：

```jsonc
{
  "pool": { "max_in_flight": 1, "max_in_flight_global": 1 },   // 并发降到 1
  "schedule": { "checkin_hours": [21], "travel_hours": [21] },  // 节奏收敛到单时点
  "upstream": { "client_version": "<跟随官方最新>", "cli_version": "<跟随官方最新>" }
}
```

再加两条运维纪律：**一次只放开一个号**（用「暂停选号」而非「禁用」，见 README §暂停选号——保号照常、流量让位），以及**新号先跑探针再入池**。

升级顺序：**收敛并发/节奏 → 对齐 UA 与设备头 → 单号轮换 → 降级到 L2/L3 全关，只留 L0。**

## 7. 退出判据与迁移（事先写下来，避免情绪化决策）

**判定"通道已死"的量化条件（满足任意两条，且持续 7 天）**：

1. L0 请求成功率 < 50%
2. 全部账号（含新登录号）均无法完成一次对话
3. 出现明确的"非官方客户端"拒绝且改 UA/头族后仍拒绝
4. 账号存活周期 < 72 小时（登录即被封）

**退出动作**：
- 冻结仓库，README 顶部改「已归档，不再修复」，保留 tag 与协议档案（**这是对社区最有价值的遗留物**）
- 资产自持：`auths/`（凭证）+ `data/state.json`（池状态）+ `config.json` 随时可备份，**不做数据锁定、不做导出收费**
- 迁移面：本网关对客户端只暴露 OpenAI 兼容接口，所以客户端侧**零改造**即可指向官方/其他兼容后端——把这一条写进 README，作为使用者的预期管理

## 8. 节奏与预算

| 周期 | 投入 | 做什么 |
|---|---|---|
| 每周 30 min | 低 | 跑一次 `doctor`，看红/黄；看面板「运行日志」异常行；确认 green commit 仍可构建 |
| 每月 2-4 h | 中 | 依赖与 Go 版本升级；`-race` + 全量测试；把当月踩的坑补进 §2.4 索引与协议档案；发一次 patch release |
| 每季 半天 | 中 | 复评 L3 各功能是否还值得存在（**主动删死代码**）；评估风控趋势与 §7 判据；写一页"继续 / 冻结 / 归档"决策记录 |
| 事件驱动 | 高 | 上游变更 → 按 §2.4 定位 → 改一处 → golden 测试 → 发布 |

**"主动删死代码"容易被忽略但很重要**：L3 的逆向功能是最脆的部分，长期不修就会腐烂成噪音（大量不可用的开关与日志）。每季度决定一批"承认放弃"，比无限期拖着一堆半坏功能更健康。

## 9. 治理边界（应对"非法转卖"）

作为接手维护者，明确写出接受/拒绝清单，**先声明，后处置**：

**接受**：L0/L1 的协议兼容修复、错误分类修正、测试与文档、可观测性、资源占用与稳定性、安全加固。

**拒绝**（不讨论、不合并）：批量注册/自动化开号、多租户与配额售卖、卡密/授权码/加壳分发、共享池对外供 API、绕过风控的对抗性增强（验证码破解、指纹伪造升级、代理池集成）。

**保留**：上游 MIT 版权声明与出处（`https://github.com/Sliverkiss/workbuddy2api`）、README 的反滥用声明、`auths/` 永远不进 git 的纪律。

理由要写明白：不是道德姿态，而是**这类功能一旦合入，维护者就被绑上了别人的商业风险**——作者停更的第三条理由，本质就是这件事把项目拖垮了。

## 10. 如果你只想自己用（最小版，只做 3 件事）

1. **装 Go、跑通 `go build/vet/test`，记录 green commit**（30 分钟）
2. **把本地逆向文档脱敏后入库**（1 小时；这一步决定你半年后还能不能修）
3. **写一个只读 `doctor` 探针 + 每日跑**（半天；这一步决定你是"追着修"还是"被告知"）

其余（CI 加固、CHANGELOG、降档预案）都可以在有需要时再补。
