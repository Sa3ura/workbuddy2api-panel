# 成长域（streak / tasks / buddy / lottery）

> **档案状态**：🟢 已校准（A 级，键位骨架直接来自真实响应）
> **最后核对**：2026-10-22 · **核对方式**：`cmd/doctor -keys`（键位骨架）+ `cmd/doctor`（8 探针全绿）
> **样本**：1 个 cn 个人号（连登 12 天、猫「滑板喵」、余额 6780/7132）
> **影响级别**：L2（签到 / 连登 / 猫猫旅行）+ L3（成长任务）

## 0. 一句话结论

成长域的所有端点在 2026-10-22 实测**均可用**，且键结构与代码基本吻合；
**唯一一处不符是 `buddy/info` 的猫标识字段**（真实名 `instance_id`，代码原写 `id`，已修）。
这一处是靠 `doctor -keys` 抓到的——正是这套工具存在的理由。

## 1. 端点

| 路径 | 方法 | 域 | 用途 | 代码 |
|---|---|---|---|---|
| `/activity/growth/streak` | GET | chatBase | 连登天数 + 档位状态 | `streak.go` `GrowthStreak` |
| `/activity/growth/redeem` | POST | chatBase | 连登档位兑换 | `GrowthRedeemTier` |
| `/activity/growth/lottery/summary` | GET | chatBase | 抽奖次数 | `LotteryChances` |
| `/activity/growth/lottery/draw` | POST | chatBase | 抽一次 | `LotteryDraw` |
| `/v2/activity/growth/tasks` | GET | chatBase | 任务列表（**带 /v2 前缀**） | `tasks.go` `ListTasks` |
| `/v2/activity/growth/tasks/accept` | POST | chatBase | 接受任务 | `AcceptTasks` |
| `/activity/growth/tasks/<code>/claim` | POST | **webBase** | 领奖（**Web 域，不是 chatBase**） | `ClaimReward` |
| `/activity/growth/buddy/info` | GET | chatBase | 猫档案 | `travel.go` `BuddyInfo` |
| `/activity/growth/buddy/travel/status` | GET | chatBase | 旅行状态 | `TravelStatus` |

## 2. 真实键位骨架（A 级，`doctor -keys` 2026-10-22）

### 2.1 `/activity/growth/streak`

```
launch_date: string
makeup_cards: { balance: number, max: number }
streak: { days, next_tier: string, next_tier_remaining, month_consumed_days,
          month_total_days, makeup_dates: array[0] }
redemption_status: {
  remaining_days, tier_7d_count, tier_7d_status: string,
  tier_14d_count, tier_14d_status: string,
  tier_28d_count, tier_28d_status: string,
  tiers: array[3] of { tier: string, days, credit, energy, cards, chances }
}
timezone: string
```

**判据要点**：
- 「已解锁可兑换」= `tier_Nd_status` 既不是 `locked` 也不是 `claimed`（连登管家就靠这个判断）；
- `tiers[]` 里 `chances` 就是该档位发放的**抽奖次数**——抽奖次数只能从兑换获得；
- `streak.days` 是「活跃上报是否真的生效」的**唯一可靠 oracle**（回读天数，不看上报的 200）。

### 2.2 `/v2/activity/growth/tasks`

```
tasks: array[N] of {
  task_code: string, title, task_desc, description,
  accept_status: string,          // not_accepted / accepted
  locked: bool, is_new: bool, is_pinned: bool,
  has_reward: bool, reward_credit, reward_energy, reward_buddy: bool,
  progress: { current: number, target: number },
  task_type: string, tag: string, jump_url: string,
  badge_name, icon_url,
  claimed_button_text, claimed_button_style, reward_popup_button_text,
  template_id_fixed: number,
  valid_start: null, valid_end: null
}
```

**实测样本**：个人号默认口径返回 **17 个任务码**，与 `probeKnownTasks`（10 个老码）命中率 **100%**。

**判据要点**：
- 任务**计分走 `/v2/report` 行为上报**，列表只反映结果 → 「上报 200 但任务不推进」是常态，必须回读此处 `progress`；
- `progress` 是**对象** `{current,target}`，不是标量（早期按标量解析会拿不到进度）；
- `accept_status` 只表示接没接，**不代表计分**。

### 2.3 `/activity/growth/buddy/info`

```
buddy: null | {
  instance_id: number,     // ← 猫的唯一标识（曾误作 id，2026-10-22 修正）
  name: string, rarity: string, personality: string, soul_desc: string,
  base_static_url, base_animated_url, full_animated_url, thumbnail_url: string,
  appearance: { hat, eye, effect, body_accessory }   // 均可能为 null
}
poll_interval_seconds: number
```

**判据要点**：
- `buddy` 为 `null` / 缺字段 / 空对象 → **无猫**（代码已归一为 nil）；
- 有猫时 `instance_id > 0`；若为 0 说明字段改名了（`probeBuddy` 会判黄）；
- 外观四件套全为 `null` 表示用默认皮肤。

### 2.4 `/activity/growth/buddy/travel/status`

```
state: string            // idle / traveling / arrived
record_id: number        // 领奖必带
buddy_id: number         // 注意：旅行状态里叫 buddy_id，与 buddy/info 的 instance_id 不同名
daily_limit_reached: bool
depart_at, arrive_at, duration_hours, server_now: number
reward_credit: number
location: { id, code, name, cover_url, duration_hours }
letter: { id, guide_text, text }
use_deeplink: string
```

**判据要点**：`daily_limit_reached` 挡住当日二次派出（自然日 CST 00:00 重置，与容器 TZ 无关）。

### 2.5 `/activity/growth/lottery/summary`

```
chances: number
module: { enabled: bool }
```

## 3. 200 ≠ 生效（本域最容易误判的地方）

| 动作 | 返回 200 但可能没生效的形态 | 唯一可信的确认方式 |
|---|---|---|
| `/v2/report` 活跃上报 | 缺 `userId` 时**静默丢弃** | 回读 `streak.days`（`checkActivityStreak` 已在做，`days=0` 记 warn） |
| `/v2/report` 任务事件链 | 客户端指纹不匹配（CLI / 桌面 / web / mp 四套） | 回读该任务的 `progress.current` |
| `tasks/accept` | 200 + OK 但不落账 | **登记回读**（任务中心已实现，失败自动重试一次） |
| `tasks/<code>/claim` | 打到错域（chatBase）会失败 | 必须在 **webBase** 且带 `x-client-platform: web` |

## 4. 已发现的漂移（时间线）

| 日期 | 发现 | 处置 |
|---|---|---|
| 2026-10-22 | `buddy.id` 恒为 0；真实键名 `instance_id` | 已修 `Buddy` 的 json tag + 补 `TestBuddyInfoMissingInstanceID`；`probeBuddy` 保留 id≤0 判黄作为长期看门狗 |

## 5. 关联的待验证项

- `UNVERIFIED.md` **#2**（上报静默丢弃）：本档只确认了 oracle 存在（`streak.days`），**上游丢弃行为本身未复现**；
- `UNVERIFIED.md` **#3**（小程序口径）：本次只测了默认 CLI 口径，mp 口径未验证；
- `UNVERIFIED.md` **#9**（补偿 `active=false` 正常）：本次样本 `active=true`（credit=500，截止 2026-10-22 23:59:59），**空分支未观测**。

## 6. 相关代码

- `internal/upstream/streak.go`、`tasks.go`、`travel.go`、`report.go`、`desktop.go`（事件链）
- 调用方：`internal/scheduler/{checkin,activity,travel,streak}.go`、`internal/panel/taskcenter.go`
- 探针：`internal/upstream/probe.go`（`tasks` / `streak` / `buddy` / `compensation`）
- 取证工具：`cmd/doctor -keys`（只导键名，不含值）
