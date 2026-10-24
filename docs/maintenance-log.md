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
### 版本串与环境坑（2026-10-22）

**版本号**：`1.13.0-panel` → **`1.13.1-panel`**。原因是本 fork 的 appVersion 与上游 v1.13.0
完全相同，部署后无法区分跑的是上游构建还是维护版，排查时抓瞎。

**版本串**：面板 `/panel/api/overview` 的 `version` 现在是 `1.13.1-panel+<commit>`
（如 `1.13.1-panel+29e5059`，工作区脏时带 `-dirty`）。实现方式：
- `-X main.buildCommit=…` 由 Makefile / build.sh / build.ps1 / Dockerfile（`ARG GIT_COMMIT`）/ CI 注入；
- 未注入时回落到 buildvcs 的 `vcs.revision`（本地直接 `go build` 的场景）；
- 兼容三种输入形态（短 sha / `git describe` 的 `-g<sha>` / 完整 40 位）——只截前 7 位会截出 tag 名。
- `cmd/doctor` 的报告同样带 commit（`doctor-1+29e5059`）；但**`go run` 不写 vcs 信息**，要 commit 得 `go build`。

**环境坑：火绒把 `-s -w` 产物判为「代码混淆器」并删除。**
现象极具误导性：`go build` 返回 0、脚本打印 "built ..."，但 `bin\wb2api.exe` **根本不存在**
（生成瞬间被杀软吃掉），而被链接的另外三个二进制（未 Strip？其实同样 Strip）当时刚好幸存，
看起来像"只漏编了一个"。
验证方法：同一源码分别带/不带 `-s -w` 各编一次，带的那份 `Test-Path` 立刻为 False。

处置：加 `-NoStrip`（PowerShell）/ `STRIP=0`（Makefile、build.sh）开关，本地调试用不剥离的产物；
发布口径（CI / Docker）仍然剥离不变。README「方式三」已写明，并建议把 `bin\` 加进杀软信任区。
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

## 首次生产部署记录（2026-10-22）

| 项 | 值 |
|---|---|
| 版本 | `1.13.2-panel+f15ee13`（tag `v1.13.2`） |
| 环境 | 阿里云 ECS，docker compose，容器 `workbuddy2api`，端口经 1Panel 反代 |
| 账号 | 3 个（cn），`green 3 / yellow 0 / red 0`，各 8 探针全绿 |
| 探针基线 | models 22（efforts=14）、balance/streak/tasks(16–17 码)/compensation(500 已领)/buddy/search(5 条) |
| 备份 | `/tmp/wb2api-backup-2026-10-22_0128.tar.gz`（84K） |
| 数据核对 | config.json md5 与备份一致；auths/ 3 个凭证无写入；data/ 属主仍 10001 |

**部署适配全部走 `.env`，服务端零文件改动**（这是 1.13.2 的主要目的）：

```
GIT_COMMIT=<hash>              # 版本串带 commit
APK_MIRROR=mirrors.aliyun.com  # dl-cdn.alpinelinux.org 在国内拉不动
BIND_ADDR=127.0.0.1            # 面板只走反代，不直接对外
PUID=10001 / PGID=10001        # 必须与 auths/、data/ 属主一致
```

**两条踩坑记录**：
1. `PUID=$(id -u)` 在 root shell 下会写成 `0`，容器随即以 root 跑并在数据目录造 root 属主文件，
   之后切回 10001 会 permission denied。要么不写这两个键（默认 10001），要么显式写 10001。
2. 体检用的 `golang:1.27`（Debian 版）镜像层在国内源常拉不动；改用 `golang:1.27-alpine` 等效
   （同为 go1.27.3），且它本来就是构建用的镜像，不额外占空间。

### 线上反馈修复（2026-10-22，1.13.3）

首次上线后用户实测报了两个 bug，都是「自动化测试覆盖不到」的那一类：

| 现象 | 根因 | 修法 |
|---|---|---|
| 健康卡片点行展开后，过几秒自动收起 | `refreshVisible()` 每 5s 轮询 → `renderCachedHealth()` → `renderHealth()` **重建 innerHTML**，`<div class="hlt-detail" hidden>` 的 hidden 被重置 | 展开状态记进 `healthExpanded`（Set），由纯函数 `healthDetailWrapHtml(acc, expanded)` 决定 hidden；再加 `healthRenderKey`，报告内容没变就不重建 DOM |
| 左下角侧栏布局被撑破（`v1.13.2-\npanel+f15ee13` 换行、`本地内存` 被挤走） | `.nav .sig .row` 只有 `display:flex`，无 `nowrap`/`ellipsis`；版本串带 commit 后从 11 字符涨到 20 字符 | 新增纯函数 `shortVer()`：去掉 `-panel`、commit 取 7 位（保留 `-dirty`），完整串进 `title`；CSS 给 `.sig .row > span` 加 `nowrap + ellipsis + min-width:0` |

**教训**：这两条都属于「DOM 状态 / 布局」类缺陷——node 测试只跑纯函数，模拟 DOM 成本高，
所以只能靠人眼在真环境里发现。因此 `shortVer` 与 `healthDetailWrapHtml` 都刻意做成**纯函数**
（可被 node harness 断言），把能测的部分从不可测的部分里切出来。

### 1.13.4：可观测性补强（2026-10-22）

上线后暴露的三处"看不见"，一次补齐：

| 项 | 问题 | 修法 |
|---|---|---|
| **容器日志无上限** | docker compose 默认 `json-file` 不设上限，长期跑会吃满宿主机磁盘（增长慢、极难联想到） | `docker-compose.yml` 加 `logging: max-size 20m × max-file 5`（上限 100MB） |
| **降级状态不可见** | 11-128 被上游通道校验拒绝后，网关把**全局** system 提示词换成中性版直到次日 00:00；用户只看到"回答风格突然变了"，无任何提示 | `degradeGate.Until()` → `Handler.DegradedUntil()` → `panel.Config.DegradedUntil` → `overview.degraded_until` → 侧栏显示「降级中 · 还剩 X」 |
| **日志留存无约定** | 容器重建即丢日志，`docker compose logs` 只能看"当前容器"，做不了跨升级的趋势统计 | README 新增「日志：上限、留存与统计口径」：升级前导出 + 用 `data/request-logs/*.jsonl` 做历史统计 |

**降级状态为什么放侧栏而不是账号池**：它影响**每一个**请求（system 提示词被整体替换），
不属于某个页面的局部状态；放侧栏才能在任何视图下都看到。

**可测性**：`degraded_until` 的后端三态（降级中 / 已过期 / 未接入）由
`TestOverviewDegradedUntil` 盯；时长文案 `degradeLeftText` 是纯函数，由
`TestAppJSDegradeLeftText` 盯 6 个用例（含脏数据与"已过期不能显示负数"）。

> 顺带记一条观察：生产环境 5 天 1434 次请求里，因 11-128 触发的重试共 4 次，**4 次全部成功**、
> 0 次 400 泄漏给客户端——渠道检测在工作，吸收机制也在工作。这个基线写进「首次生产部署记录」。

### P5 降档预案（2026-10-22）

| 交付物 | 内容 |
|---|---|
| `presets/{normal,throttle,survival}.json` | 三档**差异键清单**：`throttle` 改 11 个键（并发→1、关任务扫描与 23 点任务、巡检→24h、余额刷新→30m、排程错峰）；`survival` 只留对话 |
| `scripts/apply_preset.py` | 默认**只预览**（降档是紧张时刻的操作，最怕手滑写坏配置）；`--apply` 才写、且必留 `config.json.bak-<时间戳>`；深合并只碰预设里出现的键 → `api_key`/`auth_dir`/`models.hidden` 永不被覆盖；幂等 |
| `docs/RUNBOOK.md` | 8 节：判据表（含量化阈值）/ 降档梯子 4 级 / 切档与回滚 / **暂停选号≠禁用** / 备份恢复（含演练两版）/ 账号复活错误码表 / 日志与可观测数据口径 / 升级固定顺序 / 退出判据 |
| README | 新增「上游收紧时的降档与退出判据」节 |

**两个刻意的取舍**：
1. **默认预览不写盘**。降档发生在"已经在出事"的时刻，操作失误的代价被放大；预览把
   "改哪 11 个键、旧值→新值"全部打出来，人只需要判断对不对。
2. **预设放 `presets/`（入库）而不是 `data/presets/`**。`data/` 被 `.gitignore` 排除，
   放进去等于不存在；预设是**资产**不是运行时数据，必须能跟仓库一起走。

**为什么用暂停而不是禁用做梯子第 3 级**：禁用会让账号长期不活跃（更可疑）且停掉签到保活；
暂停只关流量、保留活跃度与积分增长。这条写进 RUNBOOK §3 的对照表。

剩余：**真恢复演练**要停机，留给运维窗口按 RUNBOOK §4 执行（不停机版可随时做）。

### 1.13.6：卡片头部溢出防护（2026-10-22）

**现象**：积分到期提醒卡的右侧按钮（收起/检查）凭空消失，快照说明文字被裁在卡片边缘。

**根因**（不是数据问题，是布局）：
- `.box > header` 是 `display:flex`，三段内容都是 `white-space: nowrap`（`h3`+`.hint`、`.note`）
- `.box` 有 `overflow: hidden`
- 当「标题说明（FEFO 那段）+ 右侧快照说明」总宽超过卡片时，flex 无法把 nowrap 文本压窄，
  于是**把后面的按钮挤出可视区并直接裁掉**——不会出现横向滚动条，所以看起来像"按钮不见了"

**修法（两层，都不靠省略号糊弄）**：
1. `button/.xs` 设 `flex: 0 0 auto` —— 按钮是最不能丢的东西，永远不参与压缩
2. `@media (max-width: 1280px)` 允许头部换行，`.grow` 在换行时隐藏（否则会把 note 顶到新行中间）
3. 兜底：`h3` / `.note` 加 `min-width:0 + overflow:hidden + text-overflow:ellipsis`

**配套**：`app.js` 把 `.hint` 文字写进 `h3.title`，窄屏被省略号截断时悬停仍能看全——
「为了不撑破布局而截断」不该变成「信息丢失」。

**这类缺陷属于自动化测试盲区**：Go 测不到 CSS，node harness 只跑纯函数。能做的只有把
布局约束写成注释（为什么这么写），以及在生产上靠人眼撞见——这次就是你撞见的。

## 客户端形态取证：官方 CLI 反查（2026-10-22，纯文档，无版本变更）

**为什么要做**：`headers.md` §10 留了个悬案——官方「设备身份三件套」里
`X-Device-Token` / `X-Device-Id-List` 我们从没发过，旧结论只能写"是否优于不发，未经验证"。
这个问题的前置条件其实是**我们模拟的到底是哪个官方客户端**：若形态是桌面端，缺两个头是残缺；
若是 CLI，不发就对齐。只靠桌面 asar 推断不出来，得看 CLI 源码。

**取证件在哪**：官方 CLI **不需要另下**——它是桌面版的出厂依赖，
落在 `D:\WorkBuddy\resources\app.asar.unpacked\cli\`（345 文件），
包名 `@tencent-ai/codebuddy-code` **2.156.0**（`publishConfig.customPackage.version`；
顶层 `version` 是 `0.0.0` 不可用），主体 `dist/codebuddy-headless.js`（11.9 MB，未混淆）。
它就是桌面端实际 spawn 的那一份，与我们 UA 的 `CLI/2.156.0` 同源。

**方法**：桌面 `app.asar` 与 CLI `dist` 各自做**字节级字面量命中统计**（只读，不发请求、不花积分）。
关键纪律：**命中 ≥1 还要看是常量定义还是赋值点**——`X-Session-ID` 就栽在这里。

| 头 | 桌面 | CLI | 结论 |
|---|---|---|---|
| `X-Device-Token` / `X-Device-Id-List` / `X-Custom-Channel-Id` | 5 / 1 / 1 | **0 / 0 / 0** | 桌面端独有 |
| `X-Machine-ID` / `X-Agent-Runtime` | 0 / 0 | 0 / 0 | 两边都没有，我们多发 |
| `X-Session-ID` | 0 | 1（常量，零赋值点） | 两边实际都没有，我们多发 |
| `X-Product` | 默认 `SaaS` | 默认 `SaaS` | 两客户端一致，只有我们给 `WorkBuddy` |
| `X-Private-Data` | 恒 `true` | `优化开 ? "false" : "true"` | 我们取桌面值 |
| `X-Agent-Type` / `X-Userinfo` | 0 | 有 | CLI 有、我们无 |

**结论一（结题）**：设备三件套是桌面端独有的，CLI 零命中。CLI 的 qimei 只从环境变量
`CODEBUDDY_QIMEI36` 读值且**只喂遥测**（缺失时 warn "telemetry qimei36 will be empty"）；
该变量由桌面 `main/qimei-helper.js` 用 `@tencent/qimei-node` 算出注入，而那份实现只分
win32/mac，**没有 linux 分支**。→ 我们不发 ②③ 不是残缺，是与 CLI 形态一致，**不必再做 A/B**。

**结论二（修正后）**：自造头里 `X-Machine-ID` / `X-Session-ID` 是两个确定的"官方永不会出现"特征；
`X-Agent-Runtime` **单列待核**——本地内置 CLI 2.156.0 零命中，但本仓库 `search.go` 注释称
「官方 2.166.0 起必发」，而 npm 最新已到 2.172.1。**取证基线存在版本代差**：
本档案锁定的是 2.156.0，任何"删头"改动前必须先用 2.172.1 重跑扫描（登记为 UNVERIFIED #18）。

**结论三（证据升级）**：`X-Product` 官方默认从"桌面端 1 份证据"变成"**桌面 + CLI 两个客户端一致**"。
待办里「唯一未对齐的静态差异」这句话现在站得住了，建议提为最高优先级。

**顺带纠正的三处陈旧记录**：
1. `headers.md` §5 的版本常量示例还写着 `5.5.4` / `2.137.1`，代码里早就是 `5.7.7` / `2.156.0`；
2. §2/§3 头族表里的 UA 实测值同样过期；
3. "`X-Machine-ID` / `X-Session-ID` 留作 A/B 开关"——**开关并不存在**，
   `injectAccountStableHeaders` 是无条件注入。真要做 A/B 得先加配置开关（约 10 行 + 测试）。

**没解决的（写进 UNVERIFIED #16/#17）**：静态逆向只答"发不发"，答不了"值长什么样"。
CLI 独立运行时到底发不发 UA 仍未确认（全文 `WorkBuddy/` 零命中，只有 axios 兜底与
UA 合并机制旁证），`X-Userinfo` / `X-Agent-Type` 的实际取值也取不到——
这些需要**动态取证**（独立装 CLI + 本地代理抓一次真实请求），留给下一轮。

**交付物**：新增 `docs/protocol/cli-client.md`（含 §10 可入库的只读复核脚本）；
更新 `headers.md` §2/§3/§5/§10、`protocol/README.md`（§1 新增「A（源码级）」证据级别 + §4 清单）、
`UNVERIFIED.md` #16/#17、`HANDOFF.md` §3/§5/§6。

## 1.13.10：版本段跟随 + `X-Product` 对齐 `SaaS`（2026-10-22）

用户更新桌面端（5.7.7 → **5.8.3**，内置 CLI 2.156.0 → **2.161.0**），触发本轮两件事。
目标是**降低「被判定为非官方客户端」的确定性**，不是"对抗"——边界见 `MAINTAINING.md` §9。

### 一、版本段过期（T0，零代码风险）

上一版 1.13.9 刚把 UA 对齐到 5.7.7 / 2.156.0，**当天下午就过期了**。UA 是风控主判据
（`headers.md` §2），旧版本号是最容易被识别的信号之一，而修正成本为零：

| 位置 | 旧 | 新 |
|---|---|---|
| `defaultClientVersion` | 5.7.7 | **5.8.3** |
| `defaultCliVersion` | 2.156.0 | **2.161.0** |

**顺带定了一条防腐烂纪律**：文档与代码注释不再写死版本号，只写**取值路径**——
`client_version` 取 `WorkBuddy.exe` 的 `VersionInfo.ProductVersion`，
`cli_version` 取 `cli/package.json` 的 `publishConfig.customPackage.version`（顶层 `version` 恒为 `0.0.0`）。
测试同理：`useragent_test.go` 里的期望串改成用常量拼，不再烤死数字（此前已经腐烂三轮：5.5.4 → 5.7.7 → 5.8.3）。

### 二、`X-Product` 默认改 `SaaS`（T1，需先解耦）

官方**两个独立客户端**的默认值都是 `SaaS`（桌面 `deploymentType?.trim() || 'SaaS'`、
CLI `… ?? "SaaS"`），我们此前默认发 `WorkBuddy` —— 唯一一处「官方两客户端都确定、只有我们不同」的静态差异。

**卡点不是改值，是解耦**：`client_name` 一个键同时决定 `X-Product` 与 `X-IDE-*` 四头，
而且 `client_name="SaaS"` 分支会把 IDE 四头整组砍掉 —— 也就是说"只想把 X-Product 改成 SaaS"做不到，
必须连 IDE 四头一起丢。所以先拆成两个维度：

| 维度 | 取值 | 配置键 |
|---|---|---|
| `X-Product` | 默认 **`SaaS`** | `upstream.product`（配 `WorkBuddy` 即回滚 1.13.9 行为） |
| `X-IDE-*` / `X-Agent-Purpose` | 默认 `WorkBuddy` | `upstream.client_name`（`SaaS` 仍是"不发 IDE 四头"的旧语义） |

兼容性：`client_name="SaaS"` 的旧语义完全保留；老配置不写 `product` 时行为变化就是"X-Product 从
WorkBuddy 变成 SaaS"，即本次的目的。回滚只需一个配置键 + 重启容器，**不需要重新构建镜像**。

### 三、一个被我撤回的改动（T2）

原计划里还有"ID 形态 32 hex → UUID"。核实时发现搞混了两条路径：桌面 `applyGatewayHeaders`
（**本机 ACP 网关**，app↔本地 agent）用 `randomUUID()` 带横线；而 **CLI → 云端 chat** 用
`generateUUUID().replace(/-/g,"")` = **32 hex**。我们发的一直是 32 hex，**本来就对齐**，改了反而错。
已把这段纠错写进 `cli-client.md` §7.1，并把 UNVERIFIED #18 更新为"2.161.0 仍零命中"。

### 验证

- `.\scripts\build.ps1 check` 全绿；新增 `attribution_test.go`（5 个解耦用例 + 默认值护栏 + 版本形状）
- `doctor -headers` 实测：`User-Agent: WorkBuddy/5.8.3 WorkBuddy/5.8.3 CLI/2.161.0`、
  `X-IDE-Version: 5.8.3`、`X-Product: SaaS`、`X-Private-Data: true`（本轮未动）
- **验收判据只能是"不劣化"**：290 请求/天 × 0.5% 基线，检出 2 倍劣化需约 16 天样本，
  本轮改动的价值是形态自洽，不是重试率下降


## 1.13.11：两处线上实案（2026-10-22）

两件事同一天撞上，**都不是 1.13.10 引入的**，都是老代码在特定触发条件下才现形。

### 一、账号池 / 模型锁池整块空白（前端）

| 项 | 内容 |
|---|---|
| 症状 | 面板「账号池」「模型锁池」两张表**只有表头没有行**；接口 `/panel/api/overview` 返回 200 且 `accounts` 有 3 个号；控制台一条 `ReferenceError: detail is not defined at rateLimitMeta (app.js:274)` |
| 根因 | `rateLimitMeta` 的两个分支写成 `detail = ...` 而**从未声明** `detail`；`app.js` 首行是 `'use strict'` → 未声明赋值抛 ReferenceError → `renderAccounts` 整块中断 |
| 为什么现在才炸 | 该函数只在账号带 `rate_limited_models`（模型级限流记录）时被调用。**必须先真的被 6004 打一次**才进入这条路径 —— 平时永远走不到 |
| 为什么看不出原因 | `loadOverview(true)` 的 `catch` 是 `if (!quiet) toast(...)`，quiet=true 时**完全静默**，前端空白无从排查 |

**修法（三处，都很小但要一起做）**：
1. `let detail;` —— 补上声明
2. 把 `renderAccounts` / `renderModelLocks` **提到**那串统计赋值之前：表格是这页的核心，不该被一个侧栏数字拖死
3. `catch` 里加 `console.error`：quiet 只决定弹不弹 toast，控制台永远留痕

**测试**：新增 `internal/panel/ratelimit_frontend_test.go`，用 node harness 在 **`'use strict'` + vm** 里跑 `rateLimitMeta` 四个分支，外加一条**变异测试**（删掉 `let detail;` 必须 FAIL）。
两个细节是这条测试的关键，写错就等于没测：
- 严格模式必须加在 `vm.runInContext` 的**内层脚本**上（外层 `.cjs` 的 `'use strict'` 不传递），否则退化成 sloppy mode，`detail = ...` 会静默建全局变量、测试全绿而 bug 照旧
- `fmtLocalDateTime` 桩必须按**本地时间**输出，不能用 `toISOString()`（会把值搬到 UTC，断言跟着错位）

### 二、6004 模型级限流以 HTTP 400 返回时被误判

| 项 | 内容 |
|---|---|
| 症状 | 单个号持续 400，`{"code":6004,"msg":"您的使用量已超出频率限制，将在 2026-10-22 22:49:09 UTC+8 重置，您也可以切换其他模型继续使用。"}`；**模型锁池是空的**；表现为「400 却切不了号」 |
| 根因 | `Classify` 只在 `status == 429` 时才归 `ErrSoftRate`；而软限流关键词表里只有 `限流`，上游写的是「频率**限制**」，匹配不上 → 一路兜到 `ErrClient` |
| 后果链 | `applyErrorPolicy` 没有 `case ErrClient` → **不冷却、不登记 `modelCooldowns`** → 模型锁池空白、同一个号下次仍被选中 → 再撞同一堵墙 |
| 修法 | `Classify` 在 429 分支后加一条：`IsModelRateLimit(body)`（只认结构化 code 6004，不看状态码）→ `ErrSoftRate`；配合已有的 `ParseRateReset`（会去掉 ` UTC+8` 后缀）精确冷却到上游给的那一刻，并写入模型锁池 → 选号时对该模型自动绕开这个号 |
| 兜底 | `softRateMarkers` 追加中文「频率限制」（无 code 的文案形态也能兜住） |

**测试**：`TestClassify` 增 4 例（400+6004 紧凑/带空格、403+6004、无 code 的中文文案），`TestParseSoftRateReset` 增 1 例（线上那条带「也可以切换其他模型」尾巴的长文案仍能解析出时间）。

### 教训（写进下一轮的习惯）

1. **`'use strict'` 下的隐式全局赋值是一类看不见的雷**。这类 bug 只在特定数据路径上触发，编译、后端测试、正常路径的手测全都不报错。用脚本扫过一遍全库（`^\s+name =` 且无同名声明），本次只有 `detail` 一处，其余 8 处是多重声明误报。
2. **静默 catch 是排障的天敌**。`quiet` 该只影响「弹不弹 UI 提示」，不该影响「留不留痕」。
3. **表格渲染要排在同一段代码的最前面**。统计数据是锦上添花，池子列表是命根子，顺序即优先级。

### 1.13.12：CSP 拦掉内联 `onsubmit`，回车提交把密钥弹窗刷掉

键盘输入 api_key 后按回车 → 控制台报
`Executing inline event handler violates the following Content Security Policy directive 'script-src 'self''`，
同时页面刷新、弹窗关闭、密钥像"没填进去"。

**根因**：`index.html` 的密钥表单写了 `onsubmit="return false"`。CSP 是 `script-src 'self'`
（刻意不留 unsafe-inline / unsafe-hashes，见 `index.go`），内联处理器被拦，
**但默认行为照常发生**——表单还是提交了，于是整页跳转。拦掉的行为不等于取消行为，
这类"报个错但照样出事"的形态比直接失效更难查。

**修法**：删掉内联属性，改由 `app.js` 给表单挂 submit 监听 `preventDefault()`。

**守卫**：新增 `TestIndexHTMLHasNoInlineEventHandlers`，扫描 `index.html` 里
「空白 + on + 字母 + =」形态的属性并失败。写这条测试时注意：**注释里出现同样字样也会被
命中**（本次就踩了），所以注释要绕开写法，别把守卫写松。

## 1.13.13：模型级限流码补 6000（2026-10-24）

**症状**：客户端收到 `429 {"code":"rate_limit_exceeded", ... message: {"code":6000,"msg":"您的使用量已超出频率限制，将在 2026-10-24 17:00:49 UTC+8 重置，您也可以切换其他模型继续使用。"}}`，
随后进池的请求回 `503 no_healthy_account`「无可用账号」——**但账号是好的**，只有那一个模型被限。

**根因**：`internal/upstream/client.go` 的 `modelRateLimitCode` 只认 **6004**。上游这次给的是 **6000**，
于是 `IsModelRateLimit` 返回 false → `applyErrorPolicy` 走**账号级** `CooldownSoftRate`，把**整个号**冷却，
而不是只锁这个模型。三个号各撞一次，池子就被冷却成空 → 后续请求选号 nil → `no_healthy_account`。

判据就在上游文案里：「**您也可以切换其他模型继续使用**」——能靠换模型绕过，就是模型级，不是账号级。

**修法**：
- `modelRateLimitCode` 常量 → `modelRateLimitCodes = {"6000", "6004"}`
- 正则改为 `(?:"code"\s*:\s*"?(?:6000|6004)"?\b)`

**`\b` 是必须的**：没有它，`6000` 会命中 `"code":60001`（计费类码），把"余额/计费"误判成"模型限流"，
进而按软限流放过一个本该硬冷却的号。测试里已加两条回归用例（60001 / 60004 必须为 false）。

**连带**：面板里的 reason 文案从 `"6004 model rate limit"` 改为 `"model rate limit (6000/6004)"`，
避免日志里明明写着 6000、面板却显示 6004。

**测试**：`TestIsModelRateLimit` +5 例（6000 紧凑/空格/字符串、60001/60004 反例）；
`TestClassify` +2 例（429+6000、400+6000 → ErrSoftRate）。

**效果**：6000 现在会写进 `modelCooldowns[model]`（对齐 17:00:49），面板「模型锁池」能看到；
账号不再被整体冷却，其它模型照常用；全池该模型都锁时回 `400 model_unavailable` 而不是「无可用账号」。

## 1.13.14：模型级限流耗尽时不再报「无可用账号」（2026-10-24）

**症状**（接 1.13.13 之后仍残留的口径问题）：撞到 6000/6004 后，若同请求内其它号被
在途占满 / 积分保底挡住 / 已在 `tried` 里，选号返回 nil → 走默认分支回
`503 no_healthy_account`「all accounts are temporarily unavailable」，客户端按
「服务端过载」反复重试，人看到「无可用账号」去查账号池（三个号全绿）。

**根因**：`modelBlock = Pool.ModelBlockedForRealm(model, realm)` 只判「**全池每个号**
都被该模型冷却」。剩下那个号没被模型冷却（只是被别的条件挡住）时它返回 `Blocked=false`，
于是落进 `no_healthy_account`。而真实语义是：**账号是好的，这个模型此刻不可用**。

**修法**（`internal/server/handler.go`）：
- 循环内记录本次失败的**性质**：`kind==ErrSoftRate && IsModelRateLimit(body)` → `modelRateHits++`
  且留住上游原文；其余错误 → `otherFailure=true`。
- 收尾时若 `!modelBlock.Blocked && modelRateHits>0 && !otherFailure` → 改回
  **400 `model_unavailable`**，hint 写清「账号健康、是模型被限流」并附上游原文。

**为什么必须带 `!otherFailure`**：只有一个「模型限流」信号、同时又混进 500 之类的
别的失败时，报 model_unavailable 会把上游故障误报成模型限流，客户端换模型也没用，
真故障被掩盖。这条写成了守卫测试。

**测试**：
- `TestChatModelRateLimitExhaustedReportsModelUnavailable`：hot 撞 6000、poor 被积分保底挡住
  → 断言 400 + `model_unavailable` + hint 含 `model_blocked` 与上游原文。
  积分保底要真正生效还需 `SetModelRateOf`（无实价台账时按目录倍率兜底，见
  `floorBlockedForRealmModel`），否则保底判据直接放行——测试里踩过一次。
- `TestChatMixedFailureKeepsNoHealthyAccount`：hot 6000 + bad 500 → 不得报 model_unavailable。
