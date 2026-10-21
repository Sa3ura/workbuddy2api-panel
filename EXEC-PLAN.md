# 分步执行计划（接管维护）

> 配套文档：`MAINTAINING.md`（策略与理由）。本文件只讲**按什么顺序做、每步怎么验收**。
> 时间估算按**一个人、每天 2-3 小时**计。

## 总览

```mermaid
flowchart LR
    P0["P0 基线<br/>0.5d"] --> P1["P1 知识资产<br/>1-2d"]
    P1 --> P2["P2 索引与记录<br/>0.5d"]
    P2 --> P3["P3 漂移探针<br/>2-3d"]
    P0 -.并行.-> P4a["P4a CI 静态检查<br/>0.5d"]
    P3 --> P5["P5 降档与运维<br/>1d"]
    P4a --> P4b["P4b race+golden<br/>1d"]
    P5 --> P6["P6 发布与治理<br/>0.5d"]
    P4b --> P6
```

**关键路径**：P0 → P1 → P2 → P3 → P5 → P6（约 5-7 天有效工时）。
**可并行**：P4a 随时可插（与 P1 并行最省时）。

**顺序不是随便排的**：
- P3 的形状断言需要"预期长什么样"——那个"预期"就来自 P1 的协议档案与 P2 的索引。先写探针会退化成"把当前行为拍成快照"，把 bug 一起固化。
- P0 必须最先：没有 green commit，后面所有改动都没有退路。

---

## P0 基线固化（0.5 天）

| # | 任务 | 具体做法 | 验收 |
|---|---|---|---|
| P0-1 | 装 Go | 装 **1.22.5**（对齐 `go.mod`）。或临时用 Docker：`docker run --rm -v %cd%:/w -w /w golang:1.23 go build ./...` | `go version` 有输出 |
| P0-2 | 跑通基线 | `go build ./...` → `go vet ./...` → `go test ./...`。**注意**：`TestAppJSTopLevelSmoke` 需要 node，没 node 会 skip/失败——先确认是 skip 而非 fail | 三条命令全绿 |
| P0-3 | 记录 green commit | `git rev-parse --short HEAD` 记进维护笔记；`git tag v1.13.0-maintain-base` | `git tag -l "*maintain*"` 有输出 |
| P0-4 | 构建脚本化 | 新建 `Makefile`（`build`/`test`/`vet`/`all`/`clean`）+ `scripts/build.ps1` + `scripts/build.sh`，把 README「方式三」4 条 `go build` 收进去 | `make all` 一条命令产出 4 个二进制 |
| P0-5 | 解除文档禁令 | `.gitignore`：删掉 `docs/` 行，把 `*.md` + `!README.md` 改为 `!docs/**/*.md`（**必须先删 `docs/`，git 不会进入被整体忽略的目录**） | `mkdir docs && echo x > docs/a.md && git status` 能看到该文件 |

**回退**：本阶段只加文件与 tag，不碰源码。任何异常 `git checkout .` 即可。

**产出**：可复现构建环境、green commit、tag、构建脚本、可提交的 docs 目录。

---

## P1 知识资产抢救（1-2 天，**最高优先**）

| # | 任务 | 具体做法 | 验收 |
|---|---|---|---|
| P1-1 | 建目录骨架 | `docs/protocol/{TEMPLATE.md, chat.md, growth-tasks.md, desktop-events.md, headers.md, search.md}` | 目录结构存在 |
| P1-2 | 搬入已有文档 | 把本地 `data/desktop-task-protocol.md`、`ANALYSIS-*.md`、`REPORT-active-map.md` 等**脱敏后**复制进 `docs/protocol/` | 文件进 git，`git log --stat` 可见 |
| P1-3 | 脱敏检查 | 扫描 `accessToken|refreshToken|Bearer|phone|nickname|uid`，逐条确认无真实值（示例值改写为 `REDACTED`） | 见 P1-5 脚本零命中 |
| P1-4 | 指针 → 内容 | 代码里 30+ 处引用外部文档的注释，先处理高频的：`ctx.md`(8)、`desktop-task-protocol.md`(3)、`buddy-adapter.ts`(3)、`sse.ts`(2)。每条改成"3 行摘要 + 落库文档路径" | 随机抽 5 处注释，其引用的文件在仓库可打开 |
| P1-5 | 密钥扫描脚本 | `scripts/check_docs_secrets.ps1`（或 `.py`），扫 `docs/` 与 `*.md`，命中即 exit 1 | 故意塞一行假 token → 脚本报错 |

**为什么要做这一步**（一句话给未来的自己）：代码里 `sse.ts:91-123` 这种引用，是指向**不在仓库里**的文件。作者的机器一旦消失，这些注释就变成无法验证的断言，上游一改你就得从零逆向。

**回退**：文档独立目录，删掉即可，不影响代码。

---

## P2 定位索引与变更记录（0.5 天）

| # | 任务 | 具体做法 | 验收 |
|---|---|---|---|
| P2-1 | `docs/TRIAGE.md` | 用 `MAINTAINING.md` §2.4 那 12 行草稿做骨架，每行补"看哪个符号 / 哪行日志" | 拿一个真实症状，能在 3 分钟内定位到文件，**不读全码** |
| P2-2 | `CHANGELOG.md` | Keep a Changelog 格式；每条写"上游变了什么 / 依据哪份档案 / 影响 L0-L3" | 补上近 3 个 tag 的条目 |
| P2-3 | 断言表去行号 | README「关键断言 ↔ 代码出处」31 条，`file.go:123` 改为 `file.go · 符号名` | 全文无 `\.go:\d+` |

**P2-3 的理由**：行号每次改动就失效，31 条错行比没有更糟。

---

## P3 漂移探针 `doctor`（2-3 天，**核心交付**）

目标：把"上游偷偷改协议 → 用户报错 → 现场逆向"反转成"探针告警 → 按需修"。

### P3-1 探针定义（`internal/upstream/probe.go`）

每个探针 = `{名称, 端点, 只读, 期望形状}`，形状校验用"字段存在性 + 集合交集"，不用深度相等（避免上游加字段就报警）。

| 探针 | 端点 | 花费 | 红（坏） | 黄（漂移） |
|---|---|---|---|---|
| refresh | `plugin/auth/token/refresh` | 免费 | 非 2xx / 无新 token | token 字段改名 |
| models | `console/enterprises/personal/models` | 免费 | 空列表 | 已知模型命中率 < 80% |
| balance | `billing/meter/get-user-resource` | 免费 | 非 2xx | 结构字段缺失 |
| streak | `activity/growth/streak` | 免费 | 非 2xx | `days` 字段缺失 |
| tasks | `activity/growth/tasks` | 免费 | 非 2xx | 任务码交集 < 80% |
| compensation | `billing/meter/compensation-status` | 免费 | 非 2xx | `active` 字段缺失 |
| buddy | `activity/growth/buddy/info` | 免费 | 非 2xx | 结构变化 |
| account | `console/account` | 免费 | 非 2xx | — |
| search | `/agenttool/v1/search` | 免费 | 非 2xx | 结果块类型变化 |
| chat（可选，默认关） | `chat/completions` `max_tokens=8` | **花积分** | 无首帧 | `finish_reason` 异常 |

**三条硬规则（写进代码注释与测试）**：
1. **绝不写**：不签到 / 不领奖 / 不上报 / 不建长会话。探针必须任何人随时可跑。
2. **形状优先于状态码**：上游改字段名照样返 200，这是最阴的变更。
3. **企业号作对照**：企业号对 growth 端点应返回 403 —— 403 就是"绿"，这是免费的契约校验。

### P3-2 命令行（`cmd/doctor/main.go`）

```powershell
go run ./cmd/doctor -config config.json            # 全账号矩阵，人类可读表
go run ./cmd/doctor -config config.json -json      # 机读
go run ./cmd/doctor -account <uid> -json           # 单号
go run ./cmd/doctor -offline                       # 只校验本地配置/文件，不打上游
```

退出码：`0` 全绿 / `1` 有黄 / `2` 有红（方便宿主 cron 与告警接入）。

### P3-3 落盘 + 面板

| # | 任务 | 做法 | 验收 |
|---|---|---|---|
| P3-3a | 输出文件 | 写 `data/upstream_health.json`（同 `output_probes.json` 的位置约定） | 文件生成，含 `generated_at` 与逐账号逐探针状态 |
| P3-3b | 面板只读透传 | 抄 `Panel.Options.ProbeFile` + `GET /panel/api/model_probes` 的模式：新增 `HealthFile` 字段 + `GET /panel/api/upstream_health`（`panel.go:193` 附近一行注册） | `curl` 带 key 能拿到 JSON |
| P3-3c | 前端（可延后） | 账号池顶部一行「上游健康：绿 8 / 黄 2 / 红 0」+ 点开详情。**建议先用 API + 日志，前端等 P5 之后** | — |
| P3-3d | 定时跑 | **推荐**：`cmd/server/main.go` 里加一个独立 ticker goroutine（约 30 行），不进调度器。理由：新 `taskKind` 会牵动 `nextWake`/`runBatch`/`Reconfigure` 与 3 处配置测试，成本 +1 天且污染既有枚举 | 重启后每日自动生成一份 health 文件 |

### P3-4 测试

| # | 任务 | 做法 | 验收 |
|---|---|---|---|
| P3-4a | 三态测试 | `probe_test.go` 用 `httptest` 造假上游，断言红/黄/绿判定 | `go test ./internal/upstream -run Probe -v` 通过 |
| P3-4b | 漂移演练 | 假上游**改一个字段名**，断言由绿转黄（不是红） | 演示一次，截日志 |

**P3 DoD**：一条命令出全账号矩阵；改字段名能判黄；全程零写操作（用上游日志或抓包自证）；`-race` 下无告警。

---

## P4 测试与 CI 加固（1-2 天，可与 P1 并行）

| # | 任务 | 做法 | 验收 |
|---|---|---|---|
| P4a-1 | 静态检查 | `.github/workflows/go-binaries.yml` 的 `test` job 里加 `go vet ./...`、`gofmt -l .`（非空即失败）、`staticcheck` | PR 上四类检查常绿 |
| P4a-2 | 漏洞扫描 | 加 `govulncheck ./...`（允许非阻塞，先观察一个月） | 有报告产出 |
| P4a-3 | 文档密钥扫描 | 把 P1-5 脚本挂进 CI | 故意提交假 token 能拦住 |
| P4b-1 | 竞态 | `go test -race ./internal/pool/... ./internal/scheduler/...` | 无 race 报告 |
| P4b-2 | golden 扩容 | `desktop.go` / `tasks.go` 的请求体做成快照测试（`sse/dsml/payload` 已有较强测试，这两块最弱） | 新增 4-6 个快照 |
| P4b-3 | 依赖 | 开 Dependabot（仅 go 模块，5 个间接依赖，摩擦极小） | 首个 PR 正常 |

**注意**：不要动现有发布纪律（`appVersion` ↔ tag 一致性断言、五平台矩阵）——那块是作者留下的好东西。

---

## P5 降档与运维（1 天）

| # | 任务 | 做法 | 验收 |
|---|---|---|---|
| P5-1 | 降档预设 | `data/presets/{normal,throttle,survival}.json`，只放**差异键**（并发、排程时点、UA 版本）。切换 = 覆盖合并 `config.json` 后用面板热生效 | 切到 throttle 后 `/status` 显示在途上限已降 |
| P5-2 | `docs/RUNBOOK.md` | 写清：降档梯子（并发↓ → 对齐 UA/设备头 → 单号轮换 → 只留 L0）、「暂停选号」正确用法（**不是禁用**）、备份/恢复、账号复活 | 照文档能独立完成一次降档与回滚 |
| P5-3 | 备份演练 | 备份 `auths/` + `data/state.json` + `config.json`，然后**真的恢复一次**（新目录起实例） | 恢复后 `/status` 账号数与积分与备份前一致 |
| P5-4 | 退出判据入 README | 4 条量化条件（L0 成功率 <50%、全号无法对话、明确"非官方客户端"拒绝且改头无效、账号存活 <72h）+ "满足两条且持续 7 天" | README 有该节 |

**P5-3 别省**：没演练过的备份等于没有备份。这一步能暴露 `auths/` 权限、`state.json` 版本字段等真问题。

---

## P6 发布与治理（0.5 天 + 持续）

| # | 任务 | 做法 | 验收 |
|---|---|---|---|
| P6-1 | `CONTRIBUTING.md` | 抄 `MAINTAINING.md` §9：接受清单 / 拒绝清单（批量开号、多租户售卖、卡密加壳、共享池、风控对抗升级） | 文件在库 |
| P6-2 | Release 流程 | 现有 CI 已断言 `appVersion` == tag；补 release note 模板（变更 / 依据档案 / 影响级别 / 降档建议） | 走通一次 patch release |
| P6-3 | 维护状态披露 | README 顶部加：「由个人接管，**只修不加**；L3 逆向功能可能长期不修」 | 声明诚实、不承诺 SLA |

---

## 时间表（建议）

| 周 | 内容 |
|---|---|
| 第 1 周 | P0（0.5d）+ P1（1.5d）+ P4a（0.5d 并行） |
| 第 2 周 | P2（0.5d）+ P3（2.5d） |
| 第 3 周 | P4b（1d）+ P5（1d）+ P6（0.5d） |
| 之后 | 每周 30 min 跑 doctor；每月一次依赖+全量测试+CHANGELOG；每季决定"继续/冻结/归档" |

## 最小可执行路径（只想自用，1 天版）

只做 3 件事，其余全部延后：

1. **P0-1 + P0-2 + P0-3**（装 Go、全绿、打 tag）— 40 min
2. **P1-1 + P1-2 + P1-3**（建目录、脱敏搬文档、扫密钥）— 2 h
3. **P3-1 + P3-2**（只读探针 + CLI，暂不做面板、不做定时）— 4 h

后两项分别解决"半年后还能不能修"和"你是追着修还是被告知"。

## 计划自身的验收（怎么知道这套东西有用）

跑一个月后回头对这四个指标，任一条不达标就说明计划需要改：

| 指标 | 目标 |
|---|---|
| 上游变更的**发现时延** | 从"用户报错"降到 **< 24h**（doctor 每日跑） |
| 单次修复**定位时间** | < 15 min（TRIAGE 命中） |
| 修复**改动面** | 单文件 + golden 测试（改不动就说明该重构） |
| L3 功能**腐烂率** | 每季度显式"承认放弃"或修好，不允许无限期半坏 |

## 已知风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| 装 Go 后测试有既有失败 | P0 卡住 | 先判断是环境（node/python）还是真失败；真失败则记录并 fix-or-skip，**不要带着红基线往下走** |
| 逆向文档拿不到 / 已丢失 | P1 无法完成 | 退化为：把注释里的引用改成"原档不可得 + 实测日期 + 结论"，先保住结论 |
| P3 形状断言把 bug 固化 | 探针假绿 | 断言只校验"字段存在 + 集合交集"，不比对具体值；用 P1 档案校准而非当前响应 |
| 上游彻底封死 | P5/P6 白做 | 这正是 P5-4 退出判据存在的意义：**知道什么时候该停**比多修一个功能重要 |
| 面板前端改动引入回归 | 面板不可用 | P3-3c 前端延后；先 API + 日志，前端单独一个 PR |
