# RUNBOOK · 降档与应急

> 这份文档只在**上游收紧**或**账号异常**时用。日常维护见 [`MAINTAINING.md`](../MAINTAINING.md)，
> 协议变更证据见 [`docs/protocol/`](./protocol/)。

## 0. 先判断：这是"上游变了"还是"我这边出问题"

| 现象 | 先查 | 结论 |
|---|---|---|
| 面板健康卡片出现**黄灯** | 点行看哪个探针黄了 + `漂移项` | 上游改了协议形状 → 按探针指向更新档案，**不必降档** |
| 健康卡片出现**红灯** | 展开看 `detail` 原文 | 接口拿不到 → 先确认网络/凭证，再考虑降档 |
| 侧栏出现「**降级中**」 | `overview.degraded_until` | 上游通道校验拒过一次 → 网关已自动换中性提示词，**观察即可** |
| 客户端报错 `content_blocked` | `data/request-logs` 里 `attempts>1` 的比例 | 反复重试仍被拒 → **降档** |
| 账号被禁用 | 面板账号池 + `/status` | 先看是不是 `11140`（账号级限制）→ **降档 + 复活流程** |

量化判据（满足任一条即降档）：

- 12 小时内 `11-128` / `content_blocked` 触发 **> 5 次**
- 客户端 400 比例 **> 5%**
- 出现 **WAF 403**（`ErrWafBlock`：403 且响应体不是业务信封）
- 任一账号被禁用

## 1. 降档梯子

**从第 1 级开始，每级至少观察 24 小时再往下走。**跳级会掩盖"到底是哪一刀起了作用"。

| 级 | 动作 | 用哪档预设 | 代价 |
|---|---|---|---|
| 1 | 并发降到 1，关掉最吵的任务（任务扫描 / 23 点任务），巡检与余额刷新拉长 | `throttle` | 积分获取变慢 |
| 2 | 再关掉 growth / travel，签到与活动错峰到非整点 | `throttle` + 手工微调 `*_hours` | 积分来源减少 |
| 3 | 只保留 1 个账号在池活跃，其余用「暂停选号」（**不是禁用**） | `survival` + 暂停 | 吞吐极低，但账号风险面最小 |
| 4 | 全部定时任务关闭，只留对话 | `survival` | 会话可能过期、积分不增长 |

## 2. 切档（一次一条命令）

```bash
cd /root/workbuddy2api-panel

# 先看要改什么（默认只预览，不写盘）
python3 scripts/apply_preset.py presets/throttle.json

# 确认无误再写（脚本会自动留 config.json.bak-<时间戳>）
python3 scripts/apply_preset.py presets/throttle.json --apply

# 验证：pool / schedule 类键热生效，不用重启
curl -sS -H "Authorization: Bearer $KEY" http://127.0.0.1:7863/panel/api/overview | head -c 200
docker compose logs --tail=30 wb2api | grep -E '上游健康巡检|排程'
```

**回滚**（同样一条命令，脚本幂等，重复执行无副作用）：

```bash
python3 scripts/apply_preset.py presets/normal.json --apply
```

> 预设只写**差异键**：`api_key`、`auth_dir`、`models.hidden` 这些个性化设置永远不会被覆盖。
> 手工改过 `config.json` 的话，切换前先跑一次预览，确认没有意外覆盖。

## 3. 「暂停选号」≠「禁用」

降档第 3 级要用暂停，不要用禁用：

| | 暂停选号 | 禁用 |
|---|---|---|
| 参与选号 | ❌ 不接对话流量 | ❌ 不接 |
| 签到 / 保活 | ✅ **照常跑** | ❌ 停 |
| 积分 | 继续增长 | 停止 |
| 恢复 | 面板一键恢复 | 需要人工介入/复活流程 |

**理由**：禁用会让账号长期不活跃，反而更容易被上游判定异常；暂停只关流量、保留活跃度。

## 4. 备份与恢复

### 备份（30 秒）

```bash
BK=/tmp/wb2api-backup-$(date +%F_%H%M)
mkdir -p "$BK"
cp -a auths data config.json "$BK"/
tar czf "$BK.tar.gz" auths data config.json
echo "备份：$BK.tar.gz ($(du -h "$BK.tar.gz" | cut -f1))"
```

要备份的只有三样：`auths/`（凭证）、`data/state.json`（池状态 + 积分基线）、`config.json`。
`data/request-logs/` 和 `data/upstream_health.json` 是**可观测数据**，丢了不影响运行。

### 恢复

```bash
docker compose down
tar xzf /tmp/wb2api-backup-<时间戳>.tar.gz -C /root/workbuddy2api-panel
chown -R 10001:10001 auths data config.json     # 属主必须与 compose 的 PUID/PGID 一致
docker compose up -d
```

### 恢复演练（**每季度做一次**，别等真出事才第一次试）

不停机版：只验证备份**可解压、可读、内容一致**。

```bash
mkdir -p /tmp/restore-test && tar xzf /tmp/wb2api-backup-<时间戳>.tar.gz -C /tmp/restore-test
ls /tmp/restore-test/auths | wc -l                       # 应等于账号数
md5sum /tmp/restore-test/config.json config.json         # 两个 md5 应相同
python3 -c "import json;d=json.load(open('/tmp/restore-test/data/state.json'));print('账号数',len(d.get('accounts',[])))"
rm -rf /tmp/restore-test
```

真恢复演练（要停机，挑没人用的时候）：

```bash
docker compose down
mv data data.broken && mv auths auths.broken            # 先挪走，别删
tar xzf /tmp/wb2api-backup-<时间戳>.tar.gz -C .
chown -R 10001:10001 auths data config.json
docker compose up -d
curl -sS -H "Authorization: Bearer $KEY" http://127.0.0.1:7863/status | head -c 300   # 账号数与积分应与备份一致
rm -rf data.broken auths.broken
```

## 5. 账号复活

被禁用（`disabled=true`）时按顺序试：

```bash
# 1) 看禁用原因（面板账号池的详情，或 /status）
# 2) 重新登录刷新 token（会覆盖 auths/<uid>.json）
docker exec -it workbuddy2api ./login
# 3) 面板恢复启用；若仍失败，看 /status 的错误码
```

| 错误码 | 含义 | 处置 |
|---|---|---|
| `11140` | 账号被上游限制（异常行为/风控） | 停用该号至少 24h，降档，别立刻重试 |
| `12153` | session 失效 | 重新登录即可 |
| `14017` | 试用未激活 | 走激活流程 |
| `11-128` | 通道校验（非官方客户端） | **不是账号问题**，别动账号，降档 |

## 6. 日志与可观测数据

| 数据 | 位置 | 跨重启 | 用途 |
|---|---|---|---|
| 容器日志 | `docker compose logs` | ❌ 重建即丢 | 排查**当前**进程 |
| 请求日志 | `data/request-logs/requests-*.jsonl` | ✅ | 历史趋势：重试率、400 比例、账号分布 |
| 健康快照 | `data/upstream_health.json` | ✅ | 最近一次探针结论（含产生它的构建 version） |

**升级前先导出**（否则历史没了）：

```bash
docker compose logs --timestamps wb2api > /var/log/wb2api-$(date +%F_%H%M).log
```

统计历史趋势用请求日志，见 README「日志：上限、留存与统计口径」。

## 7. 升级流程（固定顺序，别跳步）

```bash
cd /root/workbuddy2api-panel
docker compose logs --timestamps wb2api > /var/log/wb2api-$(date +%F_%H%M).log   # ① 留日志
cp -a auths data config.json /tmp/wb2api-bin-$(date +%F)/                        # ② 备份

git pull --ff-only origin main                                                   # ③ 取代码
sed -i "s/^GIT_COMMIT=.*/GIT_COMMIT=$(git rev-parse --short HEAD)/" .env          # ④ 版本串

docker run --rm -v "$PWD":/src -w /src \
  -e GOPROXY=https://goproxy.cn,direct -e GOTOOLCHAIN=local \
  golang:1.27-alpine sh -c 'go run ./cmd/doctor -out none'                        # ⑤ 只读体检

docker compose build                                                             # ⑥ 构建（不影响在跑的）
docker compose up -d --force-recreate                                            # ⑦ 切换
curl -sS -H "Authorization: Bearer $KEY" http://127.0.0.1:7863/panel/api/overview | head -c 200
```

体检有黄灯就**停在第 ⑤ 步**，别切。

## 8. 什么时候彻底放弃（退出判据）

以下四条**满足两条且持续 7 天**，说明这条路已不成立，应停止维护并向用户说明：

1. L0 对话成功率 < 50%
2. 所有账号都无法完成一次正常对话
3. 明确收到「非官方客户端」类拒绝，且调整请求头/提示词后仍无效
4. 新注册账号存活 < 72 小时

诚实优先：宁可早点说"这条路走不通"，也不要让用户以为只是"需要再调参数"。
