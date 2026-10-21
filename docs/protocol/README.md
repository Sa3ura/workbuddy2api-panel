# 协议档案（重建版）

本目录是网关所有「上游协议知识」的落库位置。**起点不是继承，而是重建**——先看下面的核查结论。

## 0. 结论：原始逆向文档不可找回（已核实，2026-10-21）

| 核查项 | 手段 | 结果 |
|---|---|---|
| 全历史新增文件 | `git log --all --pretty=format: --name-only --diff-filter=A` | 除 `README.md` 与 `internal/prompt/defaultprompt.md`，**没有任何 `.md` 入过库** |
| `docs/` 是否曾存在 | `git log --all --name-only -- 'docs/*'` | 空。全部 ref（含 31 个 tag、origin/upstream）均无记录 |
| 本地 `data/` | 目录检查 | 不存在（`data/` 自建仓起就在 `.gitignore` 里） |
| 同级副本 | `..\workbuddy2api-panel` | 同一个仓库，无 `data/`、无额外 `.md` |
| 磁盘搜索 | 下载目录递归搜 `desktop-task-protocol` / `ANALYSIS-*` / `buddy-adapter.ts` 等 | 无命中（个别同名 `.ts` 属其他项目，无关） |

**原因**：`.gitignore` 从一开始就排除 `docs/` 与「除 README 外的全部 `*.md`」（作者用于存放工作文档），
这些逆向记录只存在于原作者本机；上游仓库此后也被删除（见 README 顶部声明）。

**含义**：代码里 `sse.ts:91-123`、`data/desktop-task-protocol.md §7.3`、`.claude/reports/...` 这类引用，
指向的是**永久不可得**的外部材料。不是"文件被误删"，不要去找。它们的价值已经部分转移到了注释正文里
（本项目注释质量异常高，多数引用点旁边就把语义抄进了仓库）。

## 1. 事实来源分级（写档案时必须标注）

| 级别 | 来源 | 可信度 | 标注写法 |
|---|---|---|---|
| **A** | 本机网关对上游的**真实只读响应**（P3 `doctor` 探针 / 手工 curl） | 最高，可复现 | `实测 2026-10-21 · doctor:models` |
| **B** | 代码与代码注释（含原注释里引用的实测日期） | 中：可能是旧结论，上游已变 | `代码反推 · 转引注释 2026-09-12 实测 · 未复核` |
| **C** | README 断言与「关键断言 ↔ 代码出处」表 | 中低：可能与代码脱节 | `README 断言 · 未经探针复核` |

**纪律（三条，别破例）**：
1. **不得把 B 级写成实测**。转引别人的实测日期时必须注明"转引 + 未复核"。
2. 每份档案的每个判据都要带级别标注；无标注视为无效记录。
3. 引用外部文档的代码注释**保留原样**（它是历史证据），但要在同段补一行：
   `// 原始文档不可得，本文件结论见 docs/protocol/<档案>.md#<锚点>`。

## 2. 待重建清单（13 类，约 20 处引用点）

| 外部材料 | 引用位置 | 内容 | 归属档案 | 可恢复程度 |
|---|---|---|---|---|
| `data/desktop-task-protocol.md` §7.3/§8 | `upstream/desktop.go:3,300`、`panel/autotask.go:4` | 桌面事件链（RichMeow/Buddy_App/主题/模板）判据 | `desktop-events.md` | **高**：注释已列事件族与判据 |
| `docs/api-reference.md` §0/§4/§6 | `upstream/headers.go:2` | 出站头族规则 | `headers.md` | **高**：`headers.go` 通篇是头族实现 |
| `REPORT-active-map.md` §2 | `upstream/report.go:4`、`scheduler/scheduler.go:579` | 「上报 200 但静默丢弃」 | `growth-report.md` | **高** |
| `protocol.md` §7.11 | `upstream/school.go:3` | 小程序 MCP 判据（校园日/首对话） | `school-mp.md` | 中：注释有判据，缺全文 |
| `model-json-dynamic.md`（任务书） | `upstream/model_catalog.go:2`、`context_catalog.go:3`、`modelsdev.go:8` | 模型目录四级查找链 | `model-catalog.md` | **高**：代码即规范 |
| `fork-scan-absorb.md` T-1/T-2/T-3 | `upstream/transport.go:2`、`server/wafip.go:3`、`client_test.go:94` | WAF 403 归属、传输层 4 连击、429+余额措辞 | `risk-control.md` | 中 |
| `transport-hardening.md` | `upstream/transport.go:5` | 传输层加固分析 | `risk-control.md` | 中 |
| `REVIEW-conflicts-perf.md` | `pool/bench_test.go:12` | P 组性能基准结论 | `perf-baseline.md` | 低：只有基准代码，无结论 |
| `ANALYSIS-workbuddy-client-reverse.md` | `upstream/global_register.go:3` | global 注册地区完善 → trial 激活 | `global-trial.md` | 中 |
| `ANALYSIS-global-chat-solutions.md` | `upstream/headers.go:244` | 国际版域名/头族差异 | `headers.md` | 中 |
| `sse.ts:91-123,158-167`（参考仓库源码） | `upstream/tool_pairing.go:2`、`truncation.go:1` | tool 配对清理 / 残参检测语义 | `wire-expectations.md` | **高**：语义已抄进注释 |
| `buddy.ts:547-555`、`product.ts`、`buddy-adapter.ts`（参考仓库源码） | `upstream/client.go:1376,1563`、`effort_catalog.go:3,8,9,24,45`、`cache_key.go:3` | 模型可见性三类规则 / 思考档位 / `prompt_cache_key` | `model-catalog.md`、`thinking-effort.md` | **高** |

> 说明：「参考仓库源码」类（`sse.ts`/`buddy.ts`/`product.ts`/`buddy-adapter.ts`）不是本项目文档，
> 而是外部客户端实现。同样不可得，但注释里已把**语义**抄进来了，重建的成本最低。

## 3. 重建流程（P1-B）

```
① 建档骨架（本目录 + TEMPLATE.md）                       ← 已完成
② 静态抽取：按 upstream 文件抽「端点 / 必需头 / 判据 / 错误码」
   一个文件 → 一份草稿，全部标 B 级                       ← 可离线做
③ 探针校准：P3 doctor 的真实响应替换 B 级标注为 A 级       ← 需可用账号
④ 回填代码：注释里的指针补一行指向本地档案锚点            ← 边做边补
```

**顺序很重要**：没有原始文档，**探针是唯一的 ground truth**。所以 P3（漂移探针）不是 P1 的后继，
而是 P1 的校准器——两者的边界应该是：P1 出草稿，P3 出证据。

## 4. 档案清单（当前状态）

| 档案 | 级别 | 状态 |
|---|---|---|
| `TEMPLATE.md` | — | ✅ 模板 |
| `UNVERIFIED.md` | B | ✅ 待验证事实清单（探针校准的靶子） |
| `growth-report.md` | **A** | ✅ **已校准**（2026-10-22 doctor 实测：streak/tasks/buddy/lottery 键骨架 + 17 任务码 + 猫 instance_id 漂移已修） |
| `headers.md` | — | ⬜ 待建（优先，风控第一道） |
| `desktop-events.md` | — | ⬜ 待建 |
| `model-catalog.md` | **A** | ✅ **已校准**（2026-10-22 `doctor -models`：22 模型全表 + 档位/倍率/促销实测） |
| `thinking-effort.md` | **A**（形态）/ B（行为） | ✅ **已校准形态**（档位枚举与每模型默认档实测；注入/降级为代码实现） |
| `school-mp.md` | — | ⬜ 待建 |
| `wire-expectations.md` | — | ⬜ 待建 |
| `risk-control.md` | — | ⬜ 待建 |
| `global-trial.md` | — | ⬜ 待建 |
| `perf-baseline.md` | — | ⬜ 待建（优先级最低） |

## 5. 取证工具：`doctor -keys`

重建档案时最大的不确定性是「字段到底叫什么」。`cmd/doctor -keys` 对每个 growth 只读端点各发一次 GET，
导出**键位骨架**（只有键名与类型，**不含任何值**，因此可以安全入库）：

```powershell
go run ./cmd/doctor -keys > data/key-skeleton.json
```

`buddy/info` 的 `instance_id` 漂移就是这样发现的。新增端点时把它加进
`internal/upstream/probe.go` 的 `dumpGrowthKeysPaths` 即可。
