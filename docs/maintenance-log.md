# 维护日志

> 记录每次接手动过的**实质内容**与**期间发现的问题**，供后续接手/自己回看。
> 策略见 `MAINTAINING.md`，阶段计划见 `EXEC-PLAN.md`，协议事实见 `docs/protocol/`。

---

## 2026-10-21 ~ 10-22 · 接管第一轮（P0 + P1 核心 + P3）

### 背景

上游作者 2026-10-21 声明停止维护（上游开始禁反代 / 精力有限 / 非法转卖泛滥）。
本仓库为其 fork（基线 `ea8b1e5`，最后提交 `73a7560`）。本轮目标不是加功能，
而是把项目从「作者不维护 = 死掉」变成「自己还能修」。

### 累计改动

```
15 个提交 · 68 个文件 · +4198 / -117 行
基线 tag：v1.13.0-maintain-base（打在 53a4e99）
```

| 阶段 | 内容 | 提交 |
|---|---|---|
| P0 基线 | 可复现构建（便携 Go 1.22.5）、`Makefile` + `scripts/build.{ps1,sh}`、`.gitattributes`、`.gitignore` 文档解禁、gofmt 归一 | `a702c95` `2509654` `53a4e99` `05ee1a0` |
| P1 知识资产 | 核证原文档不可得 → 改为重建；`docs/protocol/` 骨架 + 5 份 A 级档案 + 25 处注释锚点 + 密钥扫描闸 + 引用守卫 | `b86c391` `0162b0a` `4958d6b` `b12474d` `8c00423` `a3c1184` |
| P3 漂移探针 | `internal/upstream/probe.go`（9 探针）+ `cmd/doctor` + 11 契约测试；面板 `GET /panel/api/upstream_health` + `POST /panel/api/health_check`、每 6h 定时巡检、账号池健康卡片（含「立即巡检」按钮） | `028b30f` `4dee69c` + 后续 |
| 修 bug | `buddy.id` → `instance_id`；补偿「已领/待领」口径；`-out` 语义；日志频道漏判 | `6c80297` `98a98f0` `e09a90e` |

### 期间发现并修复的问题

| # | 问题 | 证据 | 处置 |
|---|---|---|---|
| 1 | **`buddy/info` 的猫标识字段名错**：代码写 `json:"id"`，上游实际是 `instance_id` → ID 恒 0 | `doctor -keys` 导出的真实键骨架 | 修 tag + 加缺字段用例；探针保留 id≤0 判黄当看门狗 |
| 2 | **CRLF 陷阱**：仓库无 `.gitattributes` + `core.autocrlf=true` → `gofmt -l` 把 182 个 `.go` 全判为未格式化 | 全新 clone 后 `gofmt -l` = 0 | 加 `.gitattributes` + fmt 门禁做 CRLF 容错；Dockerfile 里的 `sed 's/\r$//'` 是同一坑遗迹 |
| 3 | **18 个文件真实格式漂移**（手工对齐字段） | `git diff --ignore-all-space` 为空 | 单独提交归一，否则 fmt 门禁对所有提交恒失败 |
| 4 | **任务类 WARN 日志进错频道**：`WARN: [panel] panel: …` 前缀挡住匹配 → 掉进「系统」频道看不见 | 新测试复现 | `classifyLine` 先剥级别前缀再匹配 |
| 5 | **探针导出泄漏账号指纹**：`sensitiveHeader` 用了非规范键名 `X-Machine-ID`，而 `http.Header` 按 `X-Machine-Id` 存取 → 原样漏出 | 首次 `-headers` 导出 | 改规范键 + `TestDumpHeaders_MasksSecrets` 兜住 |
| 6 | **补偿「有活动 credit=500」被读成「待领 500」**，实际已领（签到排程自动领的） | 实测 `claimed=true` | 探针区分 待领 / 已领取 / 无额度 |
| 7 | **PS 5.1 丢空字符串参数**：`-out '' -timeout 90s` → Go 侧收到 `-out -timeout`，报告写进名为 `-timeout` 的文件 | 仓库里真出现该文件 | 改显式哨兵 `-out none` |

### 当前可验证的状态

```
.\scripts\build.ps1 check          → gofmt + vet + 全量测试 全绿（19 个包）
python scripts\check_docs_secrets.py   → clean（11 个 md）
python scripts\check_protocol_refs.py  → clean（190 个 Go 文件，25 处引用均有锚点）
go run ./cmd/doctor                    → 8 探针全绿（真实 cn 号实测）
```

`doctor` 一次实测样本：22 个模型 / 17 个任务码 / 连登 12 天 / 余额 6780 / 猫 `instance_id` 正常 / 搜索 5 条。

### 已知边界（别高估这套东西）

- **只读探针 ≠ 防护**：它能让你提前发现通道衰减，但不能阻止上游封号或改协议。
  TLS 指纹（Go stdlib，无 uTLS）、HTTP/1.1、缺设备令牌、固定出口 IP、整点打卡式任务、
  四套客户端指纹来回切——这些结构性特征依然存在（详见对话中的风险评估）。
- 巡检每 6h 全账号清点，本身是一种**非人类规律**。在意就设 `health_check_enabled=false`
  或拉长间隔。
- 生产部署仍是上游构建；本轮所有改动**只在本地仓库**，未上线。

---

## 后续计划（按优先级）

### 待做 · 有实质价值

| 项 | 内容 | 成本 | 价值 |
|---|---|---|---|
| ✅ **升级 Go 工具链** | Go 1.22.5 → **1.27.3**；43 条标准库漏洞清零，`govulncheck` 已转阻塞（详见下「Go 升级记录」） | 已完成 | 目前唯一"确定存在"的安全缺口已关闭 |
| **P5 降档预案** | `data/presets/{normal,throttle,survival}.json` + `docs/RUNBOOK.md`（降档梯子、暂停选号用法、备份恢复）+ **真做一次恢复演练** | 1天 | 上游收紧时能 5 分钟内降档，而不是现场找参数 |
| **P1 剩余档案** | `desktop-events.md`（L3 事件链，最脆）、`school-mp.md`（活动已结束）、`risk-control.md`（部分已并入 headers §6）、`global-trial.md`（**需 global 账号**）、`perf-baseline.md` | 各 0.5-1h | 边际价值递减，按需 |
| ✅ **P4 CI 加固** | 五个守卫挂进 `.github/workflows`，本地 `check` 与 CI 同口径（`make ci`） | 已完成 | 见下「P4 记录」 |

### Go 升级记录（2026-10-22）

`govulncheck` 在 P4 接入时报 **43 条标准库漏洞**，全部来自 Go 1.22.5 的 x509 / net/http 等。
升级到当前最新稳定版 **1.27.3** 后：**No vulnerabilities found**。

改动范围（四处必须同步，否则 CI 与镜像会打架）：

| 位置 | 改动 |
|---|---|
| `go.mod` | `go 1.22.5` → `go 1.27.3`；`go mod tidy` 顺带把 `go-redis/v9` 从 indirect 纠正为 direct require（它确实被 `redisstore` 直接 import），`golang.org/x/sys` 不再单列 |
| `go.sum` | 多出 go-redis 的 test-only 依赖哈希（bsm/ginkgo、gomega、testify…），仅 go.sum 记录，不进构建 |
| `Dockerfile` | `golang:1.23-alpine` → `golang:1.27-alpine` |
| CI | 静态检查工具跟随升级：`staticcheck@2026.2.2`（旧版 2024.1.1 不认识新语言特性）、`govulncheck@v1.9.0` 且**转阻塞** |
| README | Go 徽章与「宿主机 Go ≥ 1.27.3」 |

升级时的新发现：**Go 1.27 的 gofmt 会重新对齐 `sanitize.go` 里 `sanitizeFeatures` 的尾随注释**
（CJK 注释宽度计算变了）。已本地 `gofmt -w` 应用，`git diff --ignore-all-space` 为空——纯对齐，
零语义改动。其余 182 个"未格式化"文件全是 CRLF，由容忍门禁正确忽略。

验证：`check: all green`（新工具链）、`govulncheck` 零漏洞、`staticcheck SA*` 干净、
全量 `go test ./...` 18 个包全绿。
### P4 记录（2026-10-22）

- **阻塞门禁**：`go test` / `gofmt` / `go vet` / `docs-secrets` / `protocol-refs` / `staticcheck -checks 'SA*'`
- **非阻塞（观测）**：全量 staticcheck（剩 16 条 unused 与风格）、`govulncheck`、`-race` 独立 job
- 接入时 SA* 报了 4 条，全部处理：`handler_test.go` 空 if 分支（删）、`transition_test.go` 同表达式
  （改成三次显式取值并分别断言，判据更清楚）、`client.go` chatPaths 循环无条件终止（加
  `//lint:ignore` 说明 #119 后单路径、保留 for 为复用构造）、`global_register.go` `needsRegion`
  未使用（并进报错信息，排障时能区分"提交没落账"）。
- **两个刻意的保守决定**：`-race` 首版非阻塞（本机无 gcc，无法证明接入时是绿的）；
  govulncheck 非阻塞（Go 版本问题，不是代码问题）。**没验证过的门禁不设阻塞**。

### 上线前（等决定）

| 项 | 说明 |
|---|---|
| bump `appVersion` | 现在是 `1.13.0-panel`，**与上游 v1.13.0 完全相同**——部署后无法区分跑的是哪版。建议 `1.13.1-panel` 起步 |
| `CHANGELOG.md` | 15 个提交按 新增/修复/文档 归类（P2-2，一直欠着） |
| 打 tag | CI 断言 tag == appVersion，bump 必须配套打 tag |
| 升级说明 | 新增 `schedule.health_check_*` 两键；无需迁移 `auths/`；巡检默认开启需显式关 |
| 上线顺序 | **先 `go run ./cmd/doctor` 只读体检 → 再启用定时巡检 → 最后换二进制**（换二进制要重启，会打断进行中的对话流） |

### 明确不做

- 不为批量分发 / 多租户 / 卡密售卖做任何优化（作者停更的第三条理由）
- 不追上游（已删库，无同步关系）
- 不做风控对抗性增强（验证码破解、指纹伪造升级、代理池集成）
