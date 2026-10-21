// health.go 上游健康巡检（只读漂移探针）的面板侧执行体与只读接口。
//
// 分工：
//   - 判定逻辑在 internal/upstream/probe.go（谁坏、坏在哪、形状漂不漂）
//   - 命令行观测在 cmd/doctor（人工排查、cron）
//   - 本文件负责「定时跑一遍 + 落盘 + 写面板日志 + 给前端一个只读接口」
//
// 三条纪律（与 cmd/doctor 一致，改动前先读）：
//  1. **只读**：巡检不签到 / 不领奖 / 不上报 / 不建会话 / 不刷 token，默认也不跑花积分的
//     chat 探针。所以它可以按小时级频率跑，不会消耗账号资源。
//  2. **不参与任何决策**：巡检结论只用于观测与告警，绝不进选号 / 冷却 / 熔断路径。
//     探针误判不能影响业务，否则等于给网关加了一个新的故障源。
//  3. **失败不放大**：单个账号探针失败只记一行日志，不重试、不阻塞其它账号。
package panel

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/logfmt"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

// runHealthCheckTimeout 单轮巡检的总时长上限。
// 探针本身都是短 RPC（+ 搜索一次），账号多时也够用；超时后剩余探针记 cancelled。
const runHealthCheckTimeout = 2 * time.Minute

// RunHealthCheckOnce 对池内全部账号跑一轮只读健康巡检，落盘并写日志。
//
// 包含禁用账号：禁用只表示「不参与选号」，账号本身可能仍在保号任务里跑，
// 其协议是否还对得上照样值得观测（否则某个号坏掉要等到人工发现）。
func (p *Panel) RunHealthCheckOnce() *upstream.HealthReport {
	if p.cfg.Upstream == nil || p.cfg.Pool == nil {
		return nil
	}

	accounts := make([]*auth.Auth, 0, len(p.cfg.Pool.List()))
	for _, ps := range p.cfg.Pool.List() {
		if a := p.cfg.Pool.AuthByUID(ps.UID); a != nil {
			accounts = append(accounts, a)
		}
	}
	if len(accounts) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), runHealthCheckTimeout)
	defer cancel()

	rep := upstream.RunHealthReport(ctx, p.cfg.Upstream, accounts, false, p.cfg.Version)

	// 落盘：面板 GET /panel/api/upstream_health 只读透传这个文件。
	// 写失败只告警不中断——巡检结论已经进日志了，不该因为磁盘问题丢告警。
	if p.cfg.HealthFile != "" {
		if err := upstream.SaveHealthReport(p.cfg.HealthFile, rep); err != nil {
			log.Printf("WARN: [panel] 上游健康报告写入失败：%v", err)
		}
	}

	p.logHealthReport(rep)
	return rep
}

// logHealthReport 把巡检结论写进面板日志（「任务」频道，前缀 panel: 上游健康）。
//
// 记法：全绿只一行汇总；非绿逐项一行 WARN。这样上游悄悄改协议时，
// 面板日志里能直接看到「哪个账号哪个探针、漂移项是什么」，不用去翻 JSON。
func (p *Panel) logHealthReport(rep *upstream.HealthReport) {
	if rep == nil {
		return
	}
	log.Printf("panel: 上游健康 %d 账号 · 绿 %d / 黄 %d / 红 %d · %dms",
		rep.Summary.Accounts, rep.Summary.Green, rep.Summary.Yellow, rep.Summary.Red, rep.DurationMs)

	for _, acc := range rep.Accounts {
		for _, pr := range acc.Probes {
			if pr.Status == upstream.ProbeGreen {
				continue
			}
			line := "panel: 上游健康 " + logfmt.UID8(acc.UID) + " " + pr.Name + " " + pr.Status
			if pr.Detail != "" {
				line += " · " + pr.Detail
			}
			if len(pr.Drift) > 0 {
				line += " · 漂移项: " + strings.Join(pr.Drift, ", ")
			}
			log.Printf("WARN: [panel] %s", line)
		}
	}
}

// healthRunMu 串行化手动巡检：两个浏览器同时点「立即巡检」不该并发打上游
// （每轮是 N 账号 × 9 探针，重复一轮纯属浪费，还会把时间线搅乱）。
var healthRunMu sync.Mutex

// healthCheck POST /panel/api/health_check —— 手动触发一轮巡检（同步返回结果）。
//
// 同步而非异步：账号数是个位数时一轮约 3-6 秒，返回即可渲染；
// 异步还得让前端轮询 updated_at 有没有变，多一层状态机不值当。
// 已有在跑时返回 409——前端显示「巡检中」，而不是排队再跑一遍。
func (p *Panel) healthCheck(w http.ResponseWriter, r *http.Request) {
	if p.cfg.Upstream == nil || p.cfg.Pool == nil {
		writeErr(w, http.StatusNotImplemented, "upstream/pool not available")
		return
	}
	if !healthRunMu.TryLock() {
		writeErr(w, http.StatusConflict, "health check already running")
		return
	}
	defer healthRunMu.Unlock()

	rep := p.RunHealthCheckOnce()
	if rep == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "reason": "no accounts"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "report": rep})
}

// upstreamHealth GET /panel/api/upstream_health —— 只读透传最近一次巡检报告。
//
// 语义与 model_probes 一致：文件缺失 / 未配置 = exists:false（前端显示「尚未巡检」），
// 解析失败才是错误。网关不解释报告内容，也不据此做任何决策。
func (p *Panel) upstreamHealth(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"exists": false}
	if p.cfg.HealthFile == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	raw, err := os.ReadFile(p.cfg.HealthFile)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, out)
			return
		}
		writeErr(w, http.StatusInternalServerError, "read health: "+err.Error())
		return
	}
	var rep upstream.HealthReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		writeErr(w, http.StatusBadGateway, "parse health: "+err.Error())
		return
	}
	out["exists"] = true
	out["report"] = json.RawMessage(raw)
	if fi, err := os.Stat(p.cfg.HealthFile); err == nil {
		out["updated_at"] = fi.ModTime().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}
