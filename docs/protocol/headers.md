# 出站请求头族（风控第一道）

> **档案状态**：🟢 已校准（A 级：四类头族全部由 `cmd/doctor -headers` 从真实 client 导出）
> **最后核对**：2026-10-22 · **核对方式**：`doctor -headers`（本地构造，不发请求）+ 8 个探针全绿（说明头被接受）
> **样本**：1 个 cn 个人号 · **影响级别**：L0（头族退化 = 全量被拒，最先炸的就是这里）

## 0. 为什么这份档案最重要

上游判定「请求是不是官方客户端发的」**几乎只看头**。代码里有明确证据：错误分类里有一条
`ErrChannelRefused`——`11-128 request illegal 通道校验失败（上游认为请求不是官方客户端发的）`。
也就是说头族错了，请求不是"被限流"而是"被判定为非官方客户端"，重试和换号都无效。

因此 §5 的降档梯子把「对齐 UA 与设备头」列为第二级手段。

## 1. 四类头族（实测头数：chat 28 / billing 8 / refresh 12 / search 30）

| 头族 | 用途 | 代码 |
|---|---|---|
| **chat** | 对话补全（SSE） | `CommonHeaders` + `ChatHeaders` |
| **billing** | 签到 / 余额 / 补偿 / 成长上报 | `BillingHeaders` |
| **refresh** | token 刷新 | `RefreshHeaders` |
| **search** | 联网搜索 | `SearchHeaders` |

## 2. chat 头族（实测 28 个头，完整清单）

| 头 | 实测值 / 来源 | 作用 |
|---|---|---|
| `Authorization` | `Bearer <accessToken>` | 鉴权（**安全红线：chat 永不带 `X-Refresh-Token`**） |
| `Content-Type` | `application/json` | — |
| `Accept` | `application/json, text/event-stream` | chat 覆盖 common 的非流式默认 |
| `User-Agent` | `WorkBuddy/5.5.4 WorkBuddy/5.5.4 CLI/2.137.1` | **风控主判据**（三段式） |
| `Origin` / `Referer` | `https://www.codebuddy.cn` + `/` | 域归属 |
| `X-Requested-With` | `XMLHttpRequest` | 浏览器式请求标记 |
| `Accept-Language` | `zh-CN`（global → `en-US`） | 按 realm 切，避免上游按语言缺省误判 |
| `X-CodeBuddy-Request` | `1` | **官方客户端风控闸门头，所有 API 必带** |
| `X-Machine-ID` | uid 的 `sha256("wb2a:machine:"+uid)` 前 36 hex | 账号级稳定设备标识（跨重启不变） |
| `X-Session-ID` | 同上，`wb2a:session:` | 账号级固定会话标识 |
| `X-Domain` | `www.codebuddy.cn` | 域声明 |
| `X-No-Enterprise-Id` | `1` | 个人号显式声明无企业（缺省会被疑） |
| `X-Agent-Purpose` | `conversation` | 用量归因 |
| `X-IDE-Name` / `X-IDE-Type` / `X-IDE-Product` | `WorkBuddy` | 用量归因四头 |
| `X-IDE-Version` | `5.5.4`（= `client_version`） | 用量归因 |
| `X-Product` | `WorkBuddy` | 用量归因 |
| `X-Conversation-ID` | 客户端传入值（空则**不发**） | 会话级聚合 |
| `X-Conversation-Request-ID` | 网关生成，**必发** | 对话轮级聚合主键 |
| `X-Conversation-Message-ID` | 32 hex，每条独立 | 消息级 |
| `X-Request-ID` / `X-Root-Request-ID` | 同上 | 消息级 / 根链路 |
| `X-Trace-ID` | 入站透传，空则回落 conversationRequestID | 链路追踪 |
| `X-B3-Traceid` / `X-B3-Spanid` / `X-B3-Sampled` | 32 hex / 16 hex / `1` | B3 链路头 |
| `X-Forwarded-For` / `X-Real-IP` / `X-Client-IP` | **仅 `upstream.passthrough_ip=true` 且入站有 IP** | 客户端 IP 透传 |
| `X-Device-Token` | auth 每号 > config 全局 > 文件兜底；空则**不发** | 设备风控令牌（桌面端 Turing SDK 签发） |
| `X-Enterprise-Id` / `X-Tenant-Id` | 企业号才发 | 企业归属 |

**未注入 X-Device-Token 是正常部署形态**：容器里通常没有桌面端 SDK，该头为空。
实测本机就是这样——探针全绿说明它非必需（但缺它时风控阈值可能更低，见 §5）。

## 3. billing 头族（实测 8 个）

```
Authorization / Accept / Content-Type / Accept-Language / User-Agent /
X-CodeBuddy-Request / X-User-Id / X-Domain
```

两个关键差异（都不是笔误）：

- **UA 是单段** `WorkBuddy/5.5.4`，不是 chat 的三段式——billing 域按官方 billing 客户端形态；
- **没有 Origin / Referer**：billing 域不吃这套，发了反而多余。

## 4. refresh 头族（实测 12 个）

```
Authorization 不吃；带 X-Refresh-Token（唯一携带该头的地方）/ X-Auth-Refresh-Source: plugin /
X-Machine-ID / X-Session-ID / Origin / Referer / UA 三段式 ...
```

- `X-Auth-Refresh-Source: plugin` 对齐官方客户端 refresh 渠道标识；
- **安全红线**：`X-Refresh-Token` 只出现在这里。`TestDumpHeaders_HeaderFamilies` 会断言
  chat / billing / search 都不带它——这条一旦破了就是凭证泄漏。

## 5. 版本号来源与覆盖

```go
defaultClientVersion = "5.5.4"   // UA 的 WorkBuddy/<ver> 与 X-IDE-Version
defaultCliVersion    = "2.137.1" // UA 的 CLI/<ver>
```

`upstream.client_version` / `upstream.cli_version` / `upstream.user_agent` 都能覆盖（空 = 内置默认）。
**上游改版本号时，改配置而不是改代码**——这是本项目最值得保留的设计之一。

UA 三段式：`WorkBuddy/<clientVer> <platform>/<clientVer> CLI/<cliVer>`，
其中 `<platform>` 默认 `WorkBuddy`（见 `headers.go` 的 `userAgent`）。

## 6. 已知风控信号（对应 `ErrChannelRefused` / `ErrWafBlock`）

| 信号 | 含义 | 应对 |
|---|---|---|
| 业务码 `11-128 request illegal` | **通道校验失败**：上游认为请求不是官方客户端发的 | 对齐 UA 三段式 + `X-CodeBuddy-Request` + 设备头；**不罚账号**（`ErrChannelRefused` 的语义就是"别怪账号"） |
| HTTP 403 + 非业务信封（APISIX 页） | WAF 拦截，拦的是**网关出口 IP** 而非账号 | 换出口；换号无效（`server/wafip.go` 的记录：`fork-scan-absorb T-1`） |
| HTTP 403 + `growth system is only available for personal users` | 企业号访问个人成长端点，**预期行为** | 无需处理（企业号本就无成长体系） |
| 全池同时 401/403 且刷新 token 无效 | 通道级封禁 | 见 `MAINTAINING.md` §6 降档梯子 / §7 退出判据 |

## 7. 判据与边界

- 头族**只影响出站合规性**，不进池的选号 / 冷却逻辑；
- `X-Machine-ID` / `X-Session-ID` 是 uid 派生（跨重启稳定、账号间互异），
  **不是**上游签发的真实设备令牌——与 `X-Device-Token` 是两个不同头族，官方桌面端两者都发；
- IP 透传默认关闭（`upstream.passthrough_ip=false`）：开了会把客户端真实 IP 交给上游，
  多号同机部署时这反而是负面信号。

## 8. 失效表现

| 症状 | 先看 |
|---|---|
| 大量 `11-128` / `illegal api invocation` | UA 与头族：`headers.go` 的 `userAgent` / `injectAttribution` |
| 面板「使用端」列显示异常（不是 WorkBuddy） | `upstream.user_agent` 覆盖值 |
| 官方网页「使用端」归因错 | `X-IDE-*` 四头 + `client_name`（`WorkBuddy` 才对齐桌面端） |
| 403 + HTML | 出口 IP 被 WAF 拦（`wafip.go`），不是头的问题 |
| token 刷新失败但 chat 正常 | `RefreshHeaders` 的 `X-Auth-Refresh-Source` / `X-Refresh-Token` |

## 9. 复核方式

```powershell
go run ./cmd/doctor -headers            # 敏感值已遮蔽，可直接入库
go test ./cmd/doctor/ -run DumpHeaders  # 遮蔽与头族完整性断言
```

导出覆盖：`Authorization` / `X-Refresh-Token` / `X-Device-Token` / `X-Machine-ID` / `X-Session-ID` /
`X-User-Id` / `X-Enterprise-Id` / `X-Tenant-Id` 全部遮蔽（保留长度与 4 字符头部，便于判断"有没有配"）。

---

## 10. 设备身份三件套（A 级证据：客户端源码，2026-10-22）

**证据来源**：`D:\WorkBuddy\resources\app.asar` → `/main/module.app-server.js`（203MB 包内未压缩 JS）。
WorkBuddy Desktop = 腾讯 Electron 应用，`ProductVersion 5.7.7`。

官方在每个需要身份的 API 请求上附带三个设备身份头，由同一个 provider 产出：

```js
var x = 'X-Device-Token', S = 'X-Custom-Channel-Id', ee = 'X-Device-Id-List', te = 'unknown'

C = class {
  async provide() {
    let e = { [S]: te }                                     // ① X-Custom-Channel-Id: unknown
    try {
      let t = await this.authService.getTuringDeviceToken()
      t && (e[x] = t)                                       // ② X-Device-Token: <Turing 令牌>
    } catch (e) { logger.warn('[auth/state] resolve device token failed, skip') }
    let t = process.env.CODEBUDDY_QIMEI36?.trim()
    t && (e[ee] = JSON.stringify({ qimei36: t }))           // ③ X-Device-Id-List: {"qimei36":"..."}
    return e
  }
}
```

| 头 | 取值来源 | 可复制性 | 我们的状态 |
|---|---|---|---|
| `X-Custom-Channel-Id` | **字面量 `unknown`**（常量） | ✅ 可完全照抄 | ✅ 1.13.9 起已发 |
| `X-Device-Token` | `getTuringDeviceToken()` → `turing-sdk/TuringShieldSDK.dll`（原生风控 SDK） | ❌ 无法离线合成 | ⚠️ 配置项 `upstream.device_token`，默认空 |
| `X-Device-Id-List` | `{"qimei36":"<36 位>"}`，由 `@tencent/qimei-node@1.2.2` + `qimei.dll` 生成（appKey 走 `WORKBUDDY_QIMEI_APP_KEY`） | ⚠️ 需在装客户端的机器上算 | ❌ 未发 |

**关键推论**：三条里只有 ① 是常量。②③ 依赖腾讯原生 SDK，且生成的是**那台机器的指纹**——
在 Linux 服务器上算出的 QIMEI36 与账号注册设备不一致，是否优于"不发"**未经验证**。

### 官方头构造器（同 asar，权威版本）

```js
function Jr(e) {
  let t = {}, n = e.appVersion?.trim() || '0.0.0', r = e.cliVersion?.trim()
  e.userId?.trim()       && (t['X-User-Id'] = e.userId.trim())
  e.enterpriseId?.trim() && (t['X-Enterprise-Id'] = ..., t['X-Tenant-Id'] = ...)
  e.domain?.trim()       && (t['X-Domain'] = e.domain.trim())
  t['X-Product']      = e.deploymentType?.trim() || 'SaaS'
  t['X-IDE-Type']     = 'WorkBuddy'
  t['X-IDE-Name']     = 'WorkBuddy'
  t['X-IDE-Version']  = n
  t['X-Private-Data'] = 'true'
  t['User-Agent']     = [`WorkBuddy/${n}`, `WorkBuddy/${n}`, ...(r ? [`CLI/${r}`] : [])].join(' ')
  return t
}
```

- `X-Private-Data: true` **无条件设置**（此前我们漏发，1.13.9 已补）。
- **版本段来源**：`appVersion` = 分发包版本（5.7.7）；`cliVersion` = 内置 `cli/package.json`
  的 `publishConfig.customPackage.version`（**2.156.0**；`version` 字段是 `0.0.0` 故不可用）。
  两段此前是 5.5.4 / 2.137.1，均已过期 —— 1.13.9 一并修正。
- ⚠️ 一个**仍不一致**的点：`X-Product` 官方默认是 **`SaaS`**（`deploymentType` 缺省时），
  我们默认给 `WorkBuddy`。这会改变上游用量归因的维度，**待 A/B 后再定**。

### 我们多发、官方客户端里搜不到的头

`X-Machine-ID` / `X-Session-ID`：官方 asar 全文检索**零命中**。这两个头是网关按 uid 派生的
（`deriveAccountStableID`），官方客户端用的是上面那套三件套。**多发一个官方永不发的头本身
可能是异常特征**，但也可能是上游风控"认"的东西（生产 3 号全绿、重试率 0.5%），故保留，
留作 A/B 开关。

### 复核方式

```powershell
# 只读扫描客户端包（不改安装目录）
python - <<'PY'
import re
P = r'D:\WorkBuddy\resources\app.asar'
with open(P,'rb') as f: d = f.read()
for p in [b'X-Custom-Channel-Id', b'X-Private-Data', b'getTuringDeviceToken', b'X-Machine-ID']:
    m = re.search(re.escape(p), d)
    print(p.decode(), '->', m.start() if m else 'NOT FOUND')
PY
```

对比我们自己发的头：`go run ./cmd/doctor -headers`（敏感值已遮蔽）。
