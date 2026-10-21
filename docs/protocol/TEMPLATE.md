# <主题>（如：成长任务上报）

> **档案状态**：🟡 草稿（B 级） / 🟢 已校准（A 级） / 🔴 已知失效
> **最后核对**：YYYY-MM-DD  ·  **核对方式**：`doctor:<探针名>` / 手工 curl / 仅代码阅读
> **影响级别**：L0 必须活 / L1 必须正确 / L2 尽量活 / L3 可以死（见 MAINTAINING.md §1）

## 端点

| 项 | 值 |
|---|---|
| 相对路径 | `POST /v2/report` |
| 域 | chatBase / billingBase / webBase（常量见 `internal/upstream/client.go:808`） |
| 方法 / 内容类型 | POST · `application/json` |

## 必需请求头

| 头 | 值来源 | 缺失后果 |
|---|---|---|
| `User-Agent` | `headers.go` UA 族（`WorkBuddy/<ver> … CLI/<ver>`） | 判定非官方客户端 |
| `X-Client-Platform` | `CLI` / `web` / miniprogram（**不同任务认不同指纹**） | 判据不成立，200 但不计分 |
| `Origin` / `Referer` | 按域固定 | 同上 |
| `X-User-Id` / 设备头 | `auth` 每号 / `device_token.go` | 风控 |

## 请求体

```json
[{ "event": "chat_request_send", "userId": "<uid>", "conversationId": "wb2api-<ms>" }]
```

字段说明与必填性（**必填缺失常导致 200 但静默丢弃**，见 `growth-report.md`）。

## 成功判据

| 层 | 判据 | 说明 |
|---|---|---|
| 传输层 | HTTP 200 | **不等于生效** |
| 业务层 | `body.code == 0` | |
| **真实生效** | 回读 oracle：growth 进度 / streak 天数 / 任务 progress 变化 | 唯一可信的判据 |

> **200 ≠ 生效**是本项目最容易误判的一类问题：计分异步落定、登记回读可能为空、
> 指纹不匹配时上游静默丢弃。任何标为"成功"的动作都必须写明**怎么确认它真的生效**。

## 幂等性

按天 / 按次 / 无；重复调用的后果（会不会重复扣资源、会不会覆盖进度）。

## 实测记录

| 日期 | 方式 | 结论 | 级别 |
|---|---|---|---|
| 2026-10-21 | `doctor:report` | … | A |
| 2026-09-12 | 转引注释（原文档不可得） | … | B |

## 失效表现（排障用）

| 症状 | 日志特征 | 先看哪里 |
|---|---|---|
| 上报 200 但进度不动 | `report OK but streak.days=0` | 指纹是否匹配所需事件族 |
| … | … | … |

## 相关代码

- 实现：`internal/upstream/<file>.go:<符号>`
- 调用方：`internal/scheduler/<file>.go:<符号>`
- 测试：`internal/upstream/<file>_test.go`
