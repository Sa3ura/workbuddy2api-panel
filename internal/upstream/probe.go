package upstream

// probe.go 只读漂移探针（`cmd/doctor` 的执行体）。
//
// 目的：把「上游悄悄改协议 → 用户报错 → 现场逆向」反转成「探针告警 → 按需修」。
// 本文件只负责**观测与判定**，不做任何写操作，也不改网关运行态。
//
// 三条硬规则（改动本文件前先读，见 EXEC-PLAN.md P3）：
//  1. **绝不写**：不签到 / 不领奖 / 不上报 / 不建会话 / 不刷新 token。
//     探针必须能随时随地被任何人跑，且不消耗积分、不影响账号状态。
//     唯一例外是显式开启的 chat 探针（Costy=true），它默认不跑。
//  2. **形状优先于状态码**：上游改字段名时照样返 200——这是最阴的一类变更，
//     所以判定要基于「拿到的东西是否符合预期形状」，而不只是 err == nil。
//  3. **企业版 403 是绿**：企业号没有个人成长体系（README「企业版账号」节），
//     对 growth 端点返回 403 / 400 code 10001 属**预期行为**。
//     不做这个豁免，每天都会有假告警，告警很快就没人看了。

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
)

// 三态。
const (
	// ProbeGreen 符合预期。
	ProbeGreen = "green"
	// ProbeYellow 能通但形状变了（字段缺失 / 集合交集下降）——「要变天了」的最早信号。
	ProbeYellow = "yellow"
	// ProbeRed 拿不到或明确失败。
	ProbeRed = "red"
)

// ProbeResult 单个探针的单账号结果。
type ProbeResult struct {
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Detail    string   `json:"detail,omitempty"`
	Drift     []string `json:"drift,omitempty"` // 形状漂移的具体项（缺失字段 / 缺失键）
	LatencyMs int64    `json:"latency_ms"`
	Costy     bool     `json:"costy,omitempty"`
}

// AccountHealth 单账号的全部探针结果。
type AccountHealth struct {
	UID        string        `json:"uid"`
	Realm      string        `json:"realm"`
	Enterprise bool          `json:"enterprise"`
	Status     string        `json:"status"` // 该账号最差状态（red > yellow > green）
	Probes     []ProbeResult `json:"probes"`
}

// HealthSummary 汇总。
type HealthSummary struct {
	Accounts int `json:"accounts"`
	Green    int `json:"green"`
	Yellow   int `json:"yellow"`
	Red      int `json:"red"`
}

// HealthReport 一次完整扫描的结果（落 data/upstream_health.json，面板只读透传）。
type HealthReport struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Version     string          `json:"version"`
	DurationMs  int64           `json:"duration_ms"`
	Summary     HealthSummary   `json:"summary"`
	Accounts    []AccountHealth `json:"accounts"`
}

// ExitCode red → 2，yellow → 1，全绿 → 0。供宿主 cron / 告警接入。
func (r *HealthReport) ExitCode() int {
	if r == nil {
		return 2
	}
	switch {
	case r.Summary.Red > 0:
		return 2
	case r.Summary.Yellow > 0:
		return 1
	default:
		return 0
	}
}

// probeOutcome 单探针的返回。
type probeOutcome struct {
	status string
	detail string
	drift  []string
}

func greenf(format string, args ...any) probeOutcome {
	return probeOutcome{status: ProbeGreen, detail: fmt.Sprintf(format, args...)}
}

func yellowf(drift []string, format string, args ...any) probeOutcome {
	return probeOutcome{status: ProbeYellow, detail: fmt.Sprintf(format, args...), drift: drift}
}

func redf(err error) probeOutcome {
	if err == nil {
		return probeOutcome{status: ProbeRed, detail: "failed"}
	}
	// 详情必须压成单行短句：上游 401/403 常回整页 HTML，原样铺进表格会把
	// 布局彻底打乱，也会把健康报告（面板会渲染）撑爆。
	return probeOutcome{status: ProbeRed, detail: truncateForProbe(err.Error())}
}

// probeSpec 探针定义。Run 必须是只读的（见文件头三条硬规则）。
type probeSpec struct {
	Name  string
	Costy bool
	Run   func(ctx context.Context, c *Client, a *auth.Auth) probeOutcome
}

// probeKnownTasks 稳定的老任务码，用于「任务码集合交集」漂移判定。
//
// 上游新增任务码是正常演进（不算漂移）；但**这批老码集体消失**意味着任务口径变了
// （改字段名、换域、换指纹），那是必须立刻知道的事。
// 名单随 P1 协议档案（docs/protocol/growth-report.md）校准后更新。
var probeKnownTasks = []string{
	"first_buddy",
	"chat_5",
	"template_5",
	"skill_1",
	"Buddy_App",
	"create_canvas",
	"automation_1",
	"Library_read",
	"playbook_prompt",
	"Hp_Appearance",
}

// probeKnownModels 长期存在的模型名，用于模型目录漂移判定（同 probeKnownTasks 逻辑）。
var probeKnownModels = []string{
	"deepseek-v4-flash",
	"glm-5.2",
}

// defaultProbes 默认探针集（全部只读、不花积分）。顺序即输出顺序。
func defaultProbes() []probeSpec {
	return []probeSpec{
		{Name: "account", Run: probeAccount},
		{Name: "models", Run: probeModels},
		{Name: "balance", Run: probeBalance},
		{Name: "streak", Run: probeStreak},
		{Name: "tasks", Run: probeTasks},
		{Name: "compensation", Run: probeCompensation},
		{Name: "buddy", Run: probeBuddy},
		{Name: "search", Run: probeSearch},
	}
}

// chatProbe 花积分的探针：显式开启才跑（cmd/doctor -chat）。
func chatProbe() probeSpec {
	return probeSpec{Name: "chat", Costy: true, Run: probeChat}
}

// RunHealthProbes 对单个账号跑一遍探针。
// includeChat=true 时追加 chat 探针（消耗少量积分）。
func RunHealthProbes(ctx context.Context, c *Client, a *auth.Auth, includeChat bool) AccountHealth {
	specs := defaultProbes()
	if includeChat {
		specs = append(specs, chatProbe())
	}

	h := AccountHealth{
		UID:        a.UID,
		Realm:      a.Realm(),
		Enterprise: a.IsEnterprise(),
		Status:     ProbeGreen,
		Probes:     make([]ProbeResult, 0, len(specs)),
	}

	for _, spec := range specs {
		if ctx.Err() != nil {
			h.Probes = append(h.Probes, ProbeResult{
				Name: spec.Name, Status: ProbeRed, Detail: "cancelled", Costy: spec.Costy,
			})
			continue
		}
		start := time.Now()
		out := spec.Run(ctx, c, a)
		h.Probes = append(h.Probes, ProbeResult{
			Name:      spec.Name,
			Status:    out.status,
			Detail:    out.detail,
			Drift:     out.drift,
			LatencyMs: time.Since(start).Milliseconds(),
			Costy:     spec.Costy,
		})
		if probeRank(out.status) > probeRank(h.Status) {
			h.Status = out.status
		}
	}
	return h
}

// RunHealthReport 对全部账号跑一遍探针并汇总。
func RunHealthReport(ctx context.Context, c *Client, accounts []*auth.Auth, includeChat bool, version string) *HealthReport {
	start := time.Now()
	rep := &HealthReport{
		GeneratedAt: start,
		Version:     version,
		Accounts:    make([]AccountHealth, 0, len(accounts)),
	}
	for _, a := range accounts {
		if a == nil {
			continue
		}
		h := RunHealthProbes(ctx, c, a, includeChat)
		rep.Accounts = append(rep.Accounts, h)
		rep.Summary.Accounts++
		switch h.Status {
		case ProbeRed:
			rep.Summary.Red++
		case ProbeYellow:
			rep.Summary.Yellow++
		default:
			rep.Summary.Green++
		}
	}
	rep.DurationMs = time.Since(start).Milliseconds()
	return rep
}

// probeRank 用于取最差状态。
func probeRank(status string) int {
	switch status {
	case ProbeRed:
		return 3
	case ProbeYellow:
		return 2
	case ProbeGreen:
		return 1
	default:
		return 0
	}
}

// applyProbeErr 统一的三态收口：有错误时给出结论，返回 handled=true 表示无需继续。
//
// expectUnsupported=true 表示该端点对**企业版**本就不可用（个人成长体系），
// 此时 403 / 400 code 10001 判绿并在 detail 里写明"预期"，避免每天假告警。
func applyProbeErr(a *auth.Auth, err error, expectUnsupported bool) (probeOutcome, bool) {
	if err == nil {
		return probeOutcome{}, false
	}
	if expectUnsupported && a != nil && a.IsEnterprise() && probeIsUnsupported(err) {
		return probeOutcome{status: ProbeGreen, detail: "企业版无此体系（预期）"}, true
	}
	return redf(err), true
}

// probeIsUnsupported 判定错误是否为「该账号类型不支持」形态。
//
// 两种实测形态（README「企业版账号」节，2026-10-07 A/B 对照）：
//   - growth 域：HTTP 403 "growth system is only available for personal users"
//   - billing 域：HTTP 400 code 10001「企业账号不支持该操作」
//
// 用 Status + 文案双判据：上游改文案时仍能靠 403 命中；改状态码时靠文案命中。
func probeIsUnsupported(err error) bool {
	var ue *Error
	if !errors.As(err, &ue) {
		return false
	}
	if ue.Status == 403 {
		return true
	}
	msg := strings.ToLower(ue.Msg)
	if ue.Status == 400 && (strings.Contains(msg, "10001") || strings.Contains(msg, "不支持") ||
		strings.Contains(msg, "not support") || strings.Contains(msg, "only available")) {
		return true
	}
	return strings.Contains(msg, "growth system is only available")
}

// intersectionRatio 计算 want 中有多少出现在 got 里（返回缺失项与比例）。
func intersectionRatio(want, got []string) (missing []string, ratio float64) {
	if len(want) == 0 {
		return nil, 1
	}
	have := make(map[string]struct{}, len(got))
	for _, g := range got {
		have[strings.ToLower(strings.TrimSpace(g))] = struct{}{}
	}
	hit := 0
	for _, w := range want {
		if _, ok := have[strings.ToLower(strings.TrimSpace(w))]; ok {
			hit++
		} else {
			missing = append(missing, w)
		}
	}
	sort.Strings(missing)
	return missing, float64(hit) / float64(len(want))
}

// ---------- 各探针实现（全部只读） ----------

// probeAccount 账号资料：验证 accessToken 仍有效（token 刷新链的活体检测）。
// 注意：返回的昵称属个人信息，**不落盘、不打印**。
func probeAccount(ctx context.Context, c *Client, a *auth.Auth) probeOutcome {
	nick, err := c.FetchAccountProfile(a)
	if out, done := applyProbeErr(a, err, false); done {
		return out
	}
	if strings.TrimSpace(nick) == "" {
		return yellowf([]string{"nickname"}, "200 但昵称为空（结构可能变了）")
	}
	return greenf("ok")
}

// probeModels 模型目录：空列表 = 红；长期存在的模型名集体消失 = 黄。
//
// ⚠️ 判读注意：FetchModels 会并行打企业端点与 `/v3/config`，其中 v3 一路
// **可能不校验账号**——实测用无效 token 也能拿到 19 个模型。所以 models 绿
// 只说明"目录形状没变"，**不能当成"账号可用"**；账号可用性看 account 探针。
func probeModels(ctx context.Context, c *Client, a *auth.Auth) probeOutcome {
	infos, err := c.FetchModels(a)
	if err != nil {
		return redf(err)
	}
	if len(infos) == 0 {
		return yellowf([]string{"models"}, "列表为空（上游目录可能改域/改路径）")
	}
	names := make([]string, 0, len(infos))
	for _, m := range infos {
		if m.ID != "" {
			names = append(names, m.ID)
		}
	}
	missing, ratio := intersectionRatio(probeKnownModels, names)
	if ratio < 1 {
		return yellowf(missing, "模型目录 %d 个，但熟悉的老模型缺失（命中率 %.0f%%）", len(infos), ratio*100)
	}
	return greenf("%d 个模型", len(infos))
}

// probeBalance 余额：读不到 = 红；读数全 0 但账号非新号 = 黄（可能是结构变化）。
// 只看「能不能读出结构正确的数」，不对额度本身做判断（0 额度是合法状态）。
func probeBalance(ctx context.Context, c *Client, a *auth.Auth) probeOutcome {
	remain, total, _, _, _, _, err := c.UserResourceSnapshot(a, 0)
	if err != nil {
		return redf(err)
	}
	if remain == 0 && total == 0 {
		return yellowf([]string{"credit"}, "余额读数全 0（可能确为 0，也可能字段名变了）")
	}
	return greenf("remain=%d total=%d", remain, total)
}

// probeStreak 连登：企业版预期 403（判绿）。days 本身不做判定——新号 days=0 合法。
func probeStreak(ctx context.Context, c *Client, a *auth.Auth) probeOutcome {
	days, err := c.GrowthStreak(a)
	if out, done := applyProbeErr(a, err, true); done {
		return out
	}
	return greenf("days=%d", days)
}

// probeTasks 成长任务：任务码与熟悉的老码交集低于 60% = 黄（口径可能变了）。
func probeTasks(ctx context.Context, c *Client, a *auth.Auth) probeOutcome {
	tasks, err := c.ListTasks(a)
	if out, done := applyProbeErr(a, err, true); done {
		return out
	}
	codes := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t.TaskCode != "" {
			codes = append(codes, t.TaskCode)
		}
	}
	if len(codes) == 0 {
		return yellowf([]string{"tasks"}, "任务列表为空（上游可能已停发或改口径）")
	}
	missing, ratio := intersectionRatio(probeKnownTasks, codes)
	if ratio < 0.6 {
		return yellowf(missing, "任务码 %d 个，老任务码命中率仅 %.0f%%", len(codes), ratio*100)
	}
	return greenf("%d 个任务码", len(codes))
}

// probeCompensation 补偿状态：active=false 是「当前无活动」的正常 200，不算异常。
func probeCompensation(ctx context.Context, c *Client, a *auth.Auth) probeOutcome {
	st, err := c.GetCompensationStatus(a)
	if out, done := applyProbeErr(a, err, true); done {
		return out
	}
	if !st.Active {
		return greenf("无生效活动（正常）")
	}
	return greenf("有活动 credit=%d 截止=%s", st.CreditNum, st.EndTime)
}

// probeBuddy 猫档案：未领养（buddy=nil）是合法状态，不是漂移。
func probeBuddy(ctx context.Context, c *Client, a *auth.Auth) probeOutcome {
	buddy, err := c.BuddyInfo(a)
	if out, done := applyProbeErr(a, err, true); done {
		return out
	}
	if buddy == nil {
		return greenf("未领养")
	}
	return greenf("id=%d", buddy.ID)
}

// probeSearch 联网搜索：验证 /agenttool/v1/search 仍返回结果块。
// 搜索本身不消耗账号积分（走 agenttool），仍按只读处理。
func probeSearch(ctx context.Context, c *Client, a *auth.Auth) probeOutcome {
	query := "workbuddy"
	out, err := c.Search(ctx, a, query, 1, ChatMeta{})
	if err != nil {
		return redf(err)
	}
	if out == nil || len(out.Results) == 0 {
		return yellowf([]string{"results"}, "200 但无结果（结果块结构可能变了）")
	}
	return greenf("%d 条", len(out.Results))
}

// probeChat 对话探针：**花积分**，默认不跑。
// 判据取「有没有拿到首个有效帧」，不校验正文内容（内容随模型变）。
func probeChat(ctx context.Context, c *Client, a *auth.Auth) probeOutcome {
	body := []byte(`{"model":"deepseek-v4-flash","stream":true,"max_tokens":8,` +
		`"messages":[{"role":"user","content":"hi"}]}`)
	rc, status, respBody, err := c.ChatStreamContext(ctx, a, body, "", ChatMeta{
		ConversationRequestID: fmt.Sprintf("wb2api-doctor-%d", time.Now().UnixMilli()),
	})
	if err != nil {
		return redf(err)
	}
	if rc != nil {
		defer rc.Close()
	}
	if status < 200 || status >= 300 {
		return probeOutcome{status: ProbeRed, detail: fmt.Sprintf("http %d: %s", status, truncateForProbe(string(respBody)))}
	}
	buf := make([]byte, 512)
	n, _ := rc.Read(buf)
	if n == 0 {
		return probeOutcome{status: ProbeRed, detail: "首帧为空（SSE 未按预期开始）"}
	}
	return probeOutcome{status: ProbeGreen, detail: "首帧 ok"}
}

// truncateForProbe 把任意错误体压成单行短句。
//
// 上游 401/403 常返回整页 HTML（APISIX WAF 页），直接进报告会打乱布局并撑爆面板：
// 先剥标签、再合并空白、最后截断。目的是"看得懂发生了什么"，不是保留原文。
func truncateForProbe(s string) string {
	s = stripHTMLTags(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// stripHTMLTags 去掉 <...> 片段（WAF 错误页专用，非通用 HTML 解析）。
func stripHTMLTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}
