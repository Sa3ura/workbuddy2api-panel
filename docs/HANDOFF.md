# HANDOFF · 给新会话 / 新维护者

> 一页纸交接。**新会话开场只需说一句**：「读 `docs/HANDOFF.md`，继续维护 workbuddy2api-panel」。
> 最后更新：2026-10-22（维护版 1.13.9）

## 1. 一句话现状

WorkBuddy2API Panel 是 `Sliverkiss/workbuddy2api`（已停更）的维护分支。**已上线生产**，
3 个 cn 账号全绿，探针/面板/CI/降档预案齐备，当前版本 `1.13.9-panel`。

## 2. 线上状态

| 项 | 值 |
|---|---|
| 仓库 | `https://github.com/Sa3ura/workbuddy2api-panel`（fork；upstream 指向上游只读参照） |
| 服务器 | 阿里云 ECS，`/root/workbuddy2api-panel`，docker compose，容器 `workbuddy2api`，端口经 1Panel 反代 |
| 部署版本 | 见 `panel/api/overview` 的 `version`（格式 `1.13.x-panel+<commit>`） |
| 账号 | 3 个（cn），健康卡片常绿 |
| 数据 | `auths/`、`data/`、`config.json`（都被 gitignore，**绝不入库**） |

**服务端 `.env`（部署适配，不进 git）**：

```
GIT_COMMIT=<hash>              # 让版本串带 commit
APK_MIRROR=mirrors.aliyun.com  # 国内拉不动 dl-cdn.alpinelinux.org
BIND_ADDR=127.0.0.1            # 面板只走反代
PUID=10001 / PGID=10001        # 必须与 auths/、data/ 属主一致（root 下别用 id -u！）
```

## 3. 仓库资产地图（先读这些，别通读代码）

| 文件 | 作用 |
|---|---|
| `README.md` | 项目全貌 + 面板 API + 部署运维 + 降档/退出判据 |
| `MAINTAINING.md` | 维护策略：目标 L0-L3、脆弱面地图、降级梯子 |
| `EXEC-PLAN.md` | P0-P6 分阶段计划与完成状态 |
| `docs/maintenance-log.md` | **本轮改动的完整记录**（含踩坑与依据） |
| `docs/RUNBOOK.md` | 应急手册：判据 / 降档梯子 / 切档 / 备份恢复 / 账号复活 / 升级流程 |
| `docs/protocol/` | 上游协议档案（6 份 A 级 + TEMPLATE + UNVERIFIED）；`cli-client.md` 是官方客户端形态的源码级对照 |
| `presets/` + `scripts/apply_preset.py` | 三档降档预设（normal/throttle/survival） |

## 4. 本轮（2026-10-22）交付了什么

P0 基线 / P1 档案（70%）/ P3 探针 / P4 CI 七道门禁 / P5 降档预案 全部完成；
P2（TRIAGE+CHANGELOG）、P6（CONTRIBUTING+release note）未做。

**版本演进**：1.13.0 → 1.13.9（9 个 tag），要点：

| 版本 | 内容 |
|---|---|
| 1.13.1 | 版本串带 commit（`-X main.buildCommit`） |
| 1.13.2 | 部署适配参数化（`.env`：APK_MIRROR / BIND_ADDR / GIT_COMMIT） |
| 1.13.3 | 修健康卡片展开被轮询收回 + 侧栏版本串撑破布局 |
| 1.13.4 | 降级状态可见（`overview.degraded_until`）+ 容器日志上限 |
| 1.13.5 | 降档三档预设 + `apply_preset.py` + RUNBOOK |
| 1.13.6 | 卡片头部溢出把右上按钮裁掉 |
| 1.13.7 | `apply_preset.py` 兼容 python 3.6（去掉 PEP 585 注解） |
| 1.13.8 | 缩短积分到期提醒说明文字（头部回单行） |
| 1.13.9 | 对齐官方设备身份头与版本段（客户端源码级证据） |

## 5. 已确认的事实（别再重复调研）

**WorkBuddy/CodeBuddy = 腾讯产品**，桌面端 `WorkBuddy Desktop 5.7.7`（Electron）。
客户端包在 `D:\WorkBuddy\resources\app.asar`（203MB，JS 未压缩，可直接扫）。

- 官方「设备身份三件套」：`X-Custom-Channel-Id: unknown`（常量，我们已发）、
  `X-Device-Token`（Turing Shield 原生 SDK 签发，**无法离线合成**）、
  `X-Device-Id-List: {"qimei36":...}`（腾讯 QIMEI SDK，需 appKey）
- **设备三件套是桌面端独有的**（2026-10-22 源码级证据）：官方 CLI
  （桌面版内置 `@tencent-ai/codebuddy-code` 2.156.0）对三条**全部零命中**，其 qimei 只读环境变量喂遥测。
  → 我们不发 ②③ = 与 CLI 形态一致，**不是形态残缺**。详见 `docs/protocol/cli-client.md`
- 官方 UA：`WorkBuddy/5.7.7 WorkBuddy/5.7.7 CLI/2.156.0`（我们已对齐）
- 官方 `X-Product` 默认 **`SaaS`**（**桌面端与 CLI 两个客户端一致**）；我们默认 `WorkBuddy`
  —— **唯一未对齐的静态差异，且已从"1 份证据"升级为"两客户端一致"**
- 我们多发 `X-Machine-ID` / `X-Session-ID` / `X-Agent-Runtime`（**桌面与 CLI 两个官方客户端都没有**；
  注意 `X-Session-ID` 在 CLI 里只是个从不赋值的常量，别按字符串命中误判）
- 上游 `11-128 illegal api invocation from an unapproved channel` 的发生率 ≈ **0.5%**，
  与 prompt 长度**无关**（分桶统计 p≈0.2），更像抽样式通道校验；**全部被降级重试吸收**，
  用户无感。详见 `docs/protocol/headers.md` §10

## 6. 待做（按优先级）

| 优先 | 项 | 说明 |
|---|---|---|
| 1 | **观察 1-2 周** | 1.13.9 后看 `data/request-logs` 的重试率是否下降（不变也正常） |
| 2 | `X-Product: SaaS` A/B | 唯一未对齐的静态差异；**桌面端与 CLI 默认都是 `SaaS`（两客户端一致）**，只有我们给 `WorkBuddy`。改动会影响用量归因维度 |
| 3 | P2：`TRIAGE.md` + `CHANGELOG.md` | 症状→符号索引；变更记录 |
| 4 | P6：`CONTRIBUTING.md` + release note 模板 | 计划收尾 |
| 5 | 真恢复演练 | `RUNBOOK.md` §4，需停机窗口（**至今未真跑过**） |
| 6 | P1 剩余档案 | `desktop-events.md` 最值，其余边际递减 |

## 7. 环境坑（会再踩，务必先看）

| 坑 | 症状 | 处理 |
|---|---|---|
| **火绒/杀软** | `-s -w` 产物被当"混淆器"删除，`go build` 却返回 0，二进制凭空消失 | `scripts/build.ps1 build -NoStrip` / `make STRIP=0` |
| PowerShell 5.1 | `.ps1` 含中文必须 **UTF-8 with BOM**，否则"字符串缺少终止符" | 每次编辑后补 BOM |
| PowerShell 传参 | `-o` 被当函数参数；多行中文提交信息被引号切碎 | 参数用数组；提交信息走 `git commit -F 文件` |
| 服务器 python 3.6.8 | `list[...]` 注解在函数定义时 TypeError | 脚本刻意不写类型注解 |
| CRLF | `gofmt -l` 全仓库报错 | `.gitattributes` + 容忍门禁（归一化后比较） |
| 容器日志 | 重建即丢，`docker compose logs` 只有当前容器 | 升级前先 `> /var/log/wb2api-*.log` |
| Go 版本 | 本机在 `tools\go`（1.27.3，不在 PATH），`gofmt`/`staticcheck` 需临时加 PATH | 脚本自动发现 |

## 8. 常用命令

```powershell
# 本地全量门禁（与 CI 同口径：fmt → vet → test → docs-secrets → protocol-refs）
.\scripts\build.ps1 check

# 只读体检（不碰账号状态、不花积分）
go run ./cmd/doctor -out none
go run ./cmd/doctor -headers        # 看实际出站头（敏感值已遮蔽）

# 构建（避免杀软误杀）
.\scripts\build.ps1 build -NoStrip
```

```bash
# 服务器升级（固定顺序，体检有黄灯就停在第 ⑤ 步）
docker compose logs --timestamps wb2api > /var/log/wb2api-$(date +%F_%H%M).log
cp -a auths data config.json /tmp/wb2api-bin-$(date +%F)/
git pull --ff-only origin main
sed -i "s/^GIT_COMMIT=.*/GIT_COMMIT=$(git rev-parse --short HEAD)/" .env
docker run --rm -v "$PWD":/src -w /src -e GOPROXY=https://goproxy.cn,direct \
  -e GOTOOLCHAIN=local golang:1.27-alpine sh -c 'go run ./cmd/doctor -out none'
docker compose build && docker compose up -d --force-recreate
```

## 9. 边界（作者停更的三条理由 + 我们的立场）

1. **上游封反代** → 我们只做"让个人自用更稳"，不承诺可用性
2. **精力有限** → 只修不加；新功能必须能证明"修的是已存在的问题"
3. **非法转售** → **不做**批量注册、多租户售卖、卡密加壳、共享池、风控对抗升级

退出判据见 `README.md`「上游收紧时的降档与退出判据」：四条满足两条且持续 7 天，
就如实告知"这条路不成立"，不硬撑。
