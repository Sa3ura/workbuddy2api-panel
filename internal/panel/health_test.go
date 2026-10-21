package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

// upstream_health 端点四态：文件存在 → 透传 + exists:true + updated_at；
// 未配置 / 文件缺失 → exists:false（前端显示「尚未巡检」）；文件损坏 → 502。
// 契约与 model_probes 完全同口径：网关只透传，不解析语义、不据此做任何决策。
func TestUpstreamHealthEndpoint(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "upstream_health.json")

	// 用真实结构体写盘，保证「探针产出的 JSON」与「面板能读回的 JSON」是同一份契约。
	rep := &upstream.HealthReport{
		Version:    "test",
		DurationMs: 1234,
		Summary:    upstream.HealthSummary{Accounts: 1, Green: 0, Yellow: 1, Red: 0},
		Accounts: []upstream.AccountHealth{{
			UID:    "u1",
			Realm:  "cn",
			Status: upstream.ProbeYellow,
			Probes: []upstream.ProbeResult{{
				Name: "tasks", Status: upstream.ProbeYellow,
				Detail: "老任务码命中率 0%", Drift: []string{"first_buddy"},
			}},
		}},
	}
	if err := upstream.SaveHealthReport(f, rep); err != nil {
		t.Fatal(err)
	}

	get := func(cfg Config) *httptest.ResponseRecorder {
		p := New(cfg)
		req := httptest.NewRequest("GET", "/panel/api/upstream_health", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, req)
		return rec
	}

	// 1) 文件存在：透传 + exists + updated_at
	rec := get(Config{Version: "test", APIKey: "test-key", HealthFile: f})
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Exists    bool                  `json:"exists"`
		UpdatedAt string                `json:"updated_at"`
		Report    upstream.HealthReport `json:"report"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Exists || got.UpdatedAt == "" {
		t.Fatalf("exists=%v updated_at=%q", got.Exists, got.UpdatedAt)
	}
	if got.Report.Summary.Yellow != 1 || len(got.Report.Accounts) != 1 {
		t.Fatalf("report passthrough = %+v", got.Report.Summary)
	}
	if drift := got.Report.Accounts[0].Probes[0].Drift; len(drift) != 1 || drift[0] != "first_buddy" {
		t.Fatalf("drift passthrough = %v", drift)
	}

	// 2) 文件缺失：exists:false，200（不是错误）
	rec = get(Config{Version: "test", APIKey: "test-key", HealthFile: filepath.Join(dir, "nope.json")})
	if rec.Code != http.StatusOK {
		t.Fatalf("missing file: code=%d want 200", rec.Code)
	}
	var gotMissing struct {
		Exists bool `json:"exists"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &gotMissing); err != nil {
		t.Fatal(err)
	}
	if gotMissing.Exists {
		t.Fatal("missing file: exists 应为 false")
	}

	// 3) 未配置 HealthFile：同缺失
	rec = get(Config{Version: "test", APIKey: "test-key"})
	if rec.Code != http.StatusOK {
		t.Fatalf("unconfigured: code=%d want 200", rec.Code)
	}

	// 4) 文件损坏：502（面板应显示读取失败，而不是静默空白）
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	rec = get(Config{Version: "test", APIKey: "test-key", HealthFile: bad})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("corrupt file: code=%d want 502", rec.Code)
	}
}

// 巡检日志必须落「任务」频道，否则对话流量一大会被冲掉（ring.go 前缀判定）。
func TestHealthCheckLogChannel(t *testing.T) {
	for _, line := range []string{
		"panel: 上游健康 2 账号 · 绿 1 / 黄 1 / 红 0 · 100ms",
		"WARN: [panel] panel: 上游健康 u1 tasks yellow · 老任务码命中率 0%",
	} {
		if got := classifyLine(line); got != ChTask {
			t.Errorf("%q 应归 %s 频道，got %s", line, ChTask, got)
		}
	}
}

// 探针不可用（Pool/Upstream 为 nil）时巡检必须安全返回，不能 panic——
// 巡检是观测设施，任何情况下都不该拖垮网关。
func TestRunHealthCheckOnce_NilSafe(t *testing.T) {
	p := New(Config{Version: "test"})
	if rep := p.RunHealthCheckOnce(); rep != nil {
		t.Fatalf("nil 依赖时应返回 nil，got %+v", rep.Summary)
	}
}
