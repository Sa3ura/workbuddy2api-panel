# 官方 CLI 客户端出站形态（桌面端 × CLI 对照）

> **档案状态**：🟢 已校准（**源码级 A**：客户端包逐字面量取证）
> **最后核对**：2026-10-22 · **核对方式**：只读扫描本机客户端包（`app.asar` + `app.asar.unpacked/cli/dist`），**不发请求、不改安装目录**
> **样本**：WorkBuddy Desktop `5.7.7`（Electron）+ 其内置 `@tencent-ai/codebuddy-code` `2.156.0`
> **影响级别**：L0（头族退化 = 全量被拒，最先炸的就是这里）

## 0. 这份档案回答什么问题

`headers.md` §10 记录了一个悬而未决的问题：官方「设备身份三件套」里，
`X-Device-Token` 与 `X-Device-Id-List` 我们**从来没发过**，当时只能写"是否优于不发，未经验证"。

这个问题真正的前置条件是：**我们模拟的到底是哪个官方客户端？** 两种答案会导出相反的行动：

| 若我们的形态是「官方桌面端」 | 若我们的形态是「官方 CLI」 |
|---|---|
| 缺两个设备头 = 形态残缺，应想办法补 | CLI 本来就不发 = 不发就是对齐，补反而引入桌面端独有特征 |

我们 UA 里带 `CLI/<ver>` 段（桌面端也会带，见 §5），所以**必须看 CLI 源码，不能只推断**。
本档案的结论：**设备三件套是桌面端独有的，CLI 零发送**。

## 1. 取证对象与定位

| 项 | 值 |
|---|---|
| 桌面端 | `D:\WorkBuddy\`（`WorkBuddy.exe` + `resources\app.asar` 203 MB） |
| CLI | `D:\WorkBuddy\resources\app.asar.unpacked\cli\`（345 个文件） |
| CLI 包名 / 真实版本 | `@tencent-ai/codebuddy-code` **2.156.0**（`package.json` 的 `publishConfig.customPackage.version`；顶层 `version` 字段是 `0.0.0`，不可用） |
| 包内代号 | `package.json` 的 `name` 是 `@genie/agent-cli` |
| CLI 入口 | `bin/codebuddy` / `bin/cbc`（bin 名：`codebuddy`、`codebuddy-code`、`cbc`） |
| 主要证据文件 | `cli\dist\codebuddy-headless.js`（11.9 MB，未混淆，可直读）、`cli\dist\codebuddy-lite-wb.mjs`（8.6 MB，同源另一构建） |

> **CLI 不需要另外下载**：它是桌面版的**出厂依赖**，随安装包一起落盘，并且是桌面端实际 spawn 的那一份。
> 拿它做逆向比从 npm 装一份更权威——版本号与我们 UA 里的 `CLI/2.156.0` 完全对应。

## 2. 证据表（字面量命中次数）

统计方式：**字节级出现次数**，包含常量定义、字符串比较与注释；`0` = 全文零命中。

| 头 | 桌面 `app.asar` | CLI `headless` | 结论 |
|---|---|---|---|
| `X-Device-Token` | 5 | **0** | 桌面独有 |
| `X-Device-Id-List` | 1 | **0** | 桌面独有 |
| `X-Custom-Channel-Id` | 1 | **0** | 桌面独有 |
| `X-Machine-ID` | **0** | **0** | 两个客户端都没有 |
| `X-Session-ID` | 0 | 1（仅常量，从不赋值） | 两个客户端实际都没有 |
| `X-Agent-Runtime` | **0** | **0** | ⚠️ **版本待核**（见 §11-0：本地 2.156.0 零命中，但本仓库 `search.go` 注释称「官方 2.166.0 起必发」；npm 最新已是 2.172.1） |
| `X-Product` | 10 | 2 | 都发，**默认值相同**（§5） |
| `X-Private-Data` | 2 | 1 | 都发，**取值语义不同**（§6） |
| `X-Agent-Type` | 0 | 1 | CLI 有、我们无 |
| `X-Userinfo` | 0 | 2 | CLI 有、我们无 |
| `X-No-Enterprise-Id` | 6 | 2 | 都发，我们已发 |
| `X-CodeBuddy-Request` | 2 | 1 | 都发，我们已发 |

## 3. 结论一：设备三件套是桌面端独有的

CLI 里 `X-Device-Token` / `X-Device-Id-List` / `X-Custom-Channel-Id` **全部零命中**。三条互相独立的补强证据：

**(1) CLI 的 qimei 只是遥测，不是请求头。** CLI 侧唯一相关代码是从环境变量读，且缺失时明确说只影响遥测：

```js
let tg = "CODEBUDDY_QIMEI36", tf = class {
  readEnv() {
    let L = process.env[tg]?.trim();
    if (!L) { this.missingWarned || (this.missingWarned = !0,
      this.logger.warn(`[QimeiEnvDetect] ${tg} missing in env; telemetry qimei36 will be empty for this process`)); return }
    return L;
  }
}
```

— 且 `CODEBUDDY_QIMEI36` 在 CLI 里**只喂遥测**，没有任何 `X-Device-Id-List` 的拼装点（`JSON.stringify({qimei36:...})` 这种形态在 CLI 全文不存在）。

**(2) 那个环境变量是桌面端注入的。** `app.asar.unpacked\main\qimei-helper.js` 是桌面主进程的小工具，负责算 qimei36 交给子进程：

```js
let e = process.platform === `win32`
  ? require("@tencent/qimei-node/src/win/index")
  : require("@tencent/qimei-node/src/mac/index");
let i = process.env.WORKBUDDY_QIMEI_APP_KEY?.trim() ?? ``;
if (!i) { /* 返回 {ok:false, error:"appKey missing"} */ }
let a = await new (new (r())())({ appKey: i, dllPath: process.env.WORKBUDDY_QIMEI_DLL_PATH?.trim() ?? `` }).getQimei36();
```

**(3) 该实现只分 win32 / mac 两条路径，没有 linux 分支。** 这一点对我们是决定性的：容器是 Alpine Linux，即使拿到 appKey 也无 SDK 可跑（见 §11 的取舍）。

> **对本项目的含义**：我们不发这两个头，**不是"少发了一个官方头"，而是与官方 CLI 形态一致**。
> `headers.md` §10 里"是否优于不发，未经验证"的悬案可以结题：不必验证，因为 CLI 形态下答案就是"不补"。

## 4. 结论二：双方都没有、而我们多发

| 头 | 证据 |
|---|---|
| `X-Machine-ID` | 桌面 0 命中、CLI 0 命中 |
| `X-Session-ID` | 桌面 0 命中；CLI **字面量仅 1 处**（常量赋值 `ei.SESSION_ID_HEADER="X-Session-ID"`，外加 1 处导出列表引用），**没有任何一处把它写进请求头**（无 `setHeaderCaseInsensitive(…, SESSION_ID_HEADER, …)` 调用） |
| `X-Agent-Runtime` | 桌面 0 命中、CLI（2.156.0）0 命中 —— **但结论不可靠**：`internal/upstream/search.go` 的注释明确写「官方 2.166.0 起必发，无 runtime 时回落 `"other"`」，说明**官方 CLI 已前进到 ≥2.166.0，本地这份 2.156.0 落后**（§11-0）。故本条**不能**作为"删掉它"的依据 |

这三项都是网关按 uid 派生或自造的（`deriveAccountStableID` / 固定常量），
**没有任何官方客户端会发**。它们构成我们出站形态里唯一一组"官方永不会出现"的特征。

> 注意 `X-Session-ID` 的坑：CLI 里**存在这个常量**，只看"是否命中字符串"会得出"CLI 也发"的错误结论。
> 判断头"发不发"必须看**赋值点**，不能看常量定义。

## 5. 结论三：`X-Product` 两个官方客户端默认都是 `SaaS`

CLI（`ProductEndpointHttpInterceptor`，直译）：

```js
L.headers[eu.PRODUCT] || (L.headers[eu.PRODUCT] =
  ei.getCurrentConfiguration()?.deploymentType ?? ei.configuration.getValue()?.deploymentType ?? "SaaS")
```

桌面端（`headers.md` §10 已录的 `Jr()`）：`t['X-Product'] = e.deploymentType?.trim() || 'SaaS'`。

**两个独立客户端，默认值一致且都是 `SaaS`**；`WorkBuddy` 只出现在部署类型显式配置时。
我们默认发 `WorkBuddy` —— 这是目前唯一一处「官方两个客户端都确定、只有我们不同」的静态差异。
（依据强度从"1 份证据待 A/B"升级为"2 个客户端一致、我们单方面偏离"。）

## 6. 结论四：`X-Private-Data` 两边都发，但取值语义不同

CLI（`ModelProvider` 构造请求头，直译）：

```js
let ec = await this.settingsManager.get("enableModelOptimization");
let ed = ec?.enabled !== false;              // 默认视为「开」
el[cl.PRIVATE_DATA_HEADER] = ed ? "false" : "true";
```

— 即 CLI **默认发 `X-Private-Data: false`**，只有显式关掉模型优化才发 `true`。
桌面端 `Jr()` 无条件 `t['X-Private-Data'] = 'true'`。我们 1.13.9 起发 `true`。

| 客户端 | 取值 |
|---|---|
| 桌面端 | 恒 `true` |
| CLI | `优化开启 ? "false" : "true"`（默认 `false`） |
| 我们 | `true`（= 桌面端值） |

## 7. 结论五：CLI 有、我们无的头

| 头 | CLI 取值来源 |
|---|---|
| `X-Agent-Type` | `meta.teamContext ? "team" : meta.parentConversationId ? "subagent" : "main"` |
| `X-Agent-Intent` | `meta["codebuddy.ai/mode"] ?? "craft"`（我们已发 `craft`） |
| `X-Userinfo` | 仅当 `uid && (企业 ID \|\| idSource \|\| authMethod)`：`base64(JSON{uin, owner_uin, id_source, token_source})` |
| `X-Auth-Method` / `X-Id-Source` | 来自 run identity（登录方式 / id 来源），与 `X-Userinfo` 同族 |

另外三个头 CLI **定义了常量但从不作为请求头发送**，别误判：
`X-Data-Tag`（只在一条遥测上报里用）、`X-Model-ID`、`X-Product-Version`。

## 8. 我们当前是「混合形态」，两个官方客户端都不长这样

| 维度 | 桌面端 | CLI | 我们 |
|---|---|---|---|
| UA 三段式 `WorkBuddy/<v> … CLI/<v>` | 有 | 见 §11（未确认独立运行时） | 有（桌面值 5.7.7 + 2.156.0） |
| `X-Custom-Channel-Id: unknown` | 有 | **无** | 有（桌面独有项） |
| `X-Device-Token` | 有 | **无** | 无 |
| `X-Device-Id-List` | 有 | **无** | 无 |
| `X-Machine-ID` / `X-Session-ID` / `X-Agent-Runtime` | 无 | 无 | **有** |
| `X-Private-Data` | `true` | 默认 `false` | `true` |
| `X-Agent-Type` / `X-Userinfo` | 无 | 有 | 无 |

也就是：**桌面端的 UA 与桌面端的 `X-Private-Data`、桌面独有的 `X-Custom-Channel-Id`，配上半套 CLI 头族，再加三个两边都没有的自造头。** 这个组合本身是"非官方"的直接证据，与 11-128 是否存在无关。

## 9. 对既有结论的修订（`headers.md` §10）

| 原文 | 修订 |
|---|---|
| 「`X-Machine-ID` / `X-Session-ID`：官方 asar 全文检索**零命中**」 | 桌面端零命中；**CLI 有 `X-Session-ID` 常量但从不赋值**；`X-Agent-Runtime` 两边都零命中（新增） |
| 「设备三件套里只有①可完全照抄」「②③需腾讯原生 SDK，是否优于不发未经验证」 | ②③ 的结论改成**结构性结论**：它们是桌面端独有头，CLI 零发送 → 我们不发即对齐 CLI，不必再做 A/B |
| §5 版本常量示例 `5.5.4` / `2.137.1` | 代码实际默认已是 `5.7.7` / `2.156.0`（`headers.go` 的 `defaultClientVersion` / `defaultCliVersion`） |

## 10. 复核方式（只读，可安全入库，不含任何真实值）

```python
import re
paths = {
    "desktop": r"D:\WorkBuddy\resources\app.asar",
    "cli":     r"D:\WorkBuddy\resources\app.asar.unpacked\cli\dist\codebuddy-headless.js",
}
heads = ["X-Device-Token", "X-Device-Id-List", "X-Custom-Channel-Id", "X-Machine-ID",
         "X-Session-ID", "X-Agent-Runtime", "X-Product", "X-Private-Data",
         "X-Agent-Type", "X-Userinfo", "X-No-Enterprise-Id", "X-CodeBuddy-Request"]
data = {k: open(v, "rb").read() for k, v in paths.items()}
print(f"{'header':22}" + "".join(f"{k:10}" for k in paths))
for h in heads:
    print(f"{h:22}" + "".join(f"{data[k].count(h.encode()):<10}" for k in paths))

# 「有没有赋值」比「有没有出现」重要：查行内 setHeader / headers[...] = 的写法
d = data["cli"].decode("utf-8", "replace")
for h in ["X-Session-ID", "X-Agent-Type", "X-Private-Data"]:
    print(h, "→", d.count(h), "处出现")
```

判据：**命中数 0 = 该客户端不发**；命中 ≥1 时还要看它出现在**常量定义**还是**赋值/`set` 点**（§4 的坑）。

## 11. 局限与未验证项

**0. 版本代差（最重要的一条，先看这个）。** 本档案取证的是**桌面端内置 CLI `2.156.0`**，
而 npm 上 `@tencent-ai/codebuddy-code` 最新已到 **`2.172.1`**（2026-10-22 查 registry）。
本仓库 `internal/upstream/search.go` 的注释还写着「`X-Agent-Runtime`：官方 **2.166.0** 起必发」——
即**官方 CLI 已经越过 2.156.0 好几个版本，期间新增过头**。

因此本档案的每条结论都要分两类看：

| 类型 | 例子 | 是否受版本代差影响 |
|---|---|---|
| **结构性结论**（客户端形态的定位） | 设备三件套是否属于桌面端；`X-Machine-ID` 这种自造头是否官方有 | 低：形态定位不会因为几个 minor 版本翻盘 |
| **逐头结论**（某个头某版本发不发） | `X-Agent-Runtime`、`X-Agent-Type`、`X-Userinfo` | **高：必须按最新版复核**，否则会把"新版新增的头"当成"官方不发" |

**行动**：任何"删除我们多发的头"的改动，必须先拉 `2.172.1` 重跑 §10 的脚本（只读、约十分钟）。
在那之前删 `X-Agent-Runtime` 是**有证据反向支持的危险动作**。

1. **静态逆向只回答"发不发"，回答不了"值长什么样"。** `X-Userinfo`（base64 内容）、`X-Agent-Type` 的实际取值、
   `X-Private-Data` 的真实默认，都需要**动态取证**（独立安装 CLI + 本地代理抓一次真实请求）才能确认。本档案不覆盖。
2. **CLI 独立运行时是否设置 UA，未确认。** CLI 全文 `WorkBuddy/` **0 命中**，唯一的兜底是 axios 库内的
   `User-Agent: axios/<ver>`；UA 也可能由某个 interceptor / `userAgentExtension`（`X-Skip-User-Agent-Merge` 旁证存在 UA 合并机制）注入。
   **本轮未定位到注入点 = 未取得证据，不等于 CLI 不发 UA** → 标 **B/未验**，列入 `UNVERIFIED.md` 待办。
3. CLI 有两份构建（`headless` 与 `lite-wb`），本档案以 `headless` 为准；两者在头名上一致（同一份源码的两个 target）。
4. 桌面端证据来自 `app.asar`（JS）；`native\turing-sdk` 为原生二进制，**不在文本扫描范围内**，
   所以"桌面端发 `X-Device-Token`"的依据是 JS 侧拼装点，而非 SDK 内部行为。

## 12. 对本项目决策的影响

| 议题 | 原先状态 | 本档案之后 |
|---|---|---|
| ① `X-Device-Id-List`（QIMEI36） | "收益未证、是否优于不发没证据" | **不做**，且理由升级：CLI 零发送 = 不发即对齐；且 SDK 无 linux 分支 |
| ② `X-Device-Token` | "无法离线合成" | **不做**，理由不变（桌面独有 + 按设备签发，共用=多号关联） |
| ③ 删 `X-Machine-ID` / `X-Session-ID` / `X-Agent-Runtime` | 待 A/B 的 2 个头 | `X-Machine-ID` / `X-Session-ID` 定论：桌面与 CLI（2.156.0）都没有，性质为"官方永不会出现的特征"；`X-Agent-Runtime` **单列待核**（可能正是 2.166.0+ 的新增头，删之前必须按最新版复核，见 §11-0） |
| `X-Product: SaaS` | 待办 #2（1 份证据） | 依据升级为两客户端一致，**建议提为最高优先级** |
| 补 `X-Agent-Type` / `X-Userinfo` | 未列入 | 新增候选（CLI 有、我们无），**需先定形态再补**：只补头不改 UA，混合形态问题（§8）不会消失 |

## 相关代码与档案

- 我们的实现：`internal/upstream/headers.go`（`defaultWorkBuddyUAFor` / `injectChannelID` / `injectAccountStableHeaders` / `resolveDeviceToken`）
- 我们的自造头：`internal/upstream/headers.go` 的 `deriveAccountStableID`
- 关联档案：`docs/protocol/headers.md` §10（设备身份三件套）、`docs/protocol/UNVERIFIED.md`（待验证清单）
- 取证脚本：本档案 §10（只读，无需账号，不花积分）
