package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/pool"
)

// 降级状态透出（1.13.4）。
//
// 为什么值得单测：降级会把**全局** system 提示词换成中性版直到次日 00:00 CST。
// 这个状态以前只能靠翻日志推断或重启清掉——用户只会觉得"回答风格突然变了"。
// 三个分支必须都对，否则面板要么永远不提示，要么一直显示一个早就过期的"降级中"。
func TestOverviewDegradedUntil(t *testing.T) {
	overview := func(cfg Config) map[string]any {
		t.Helper()
		p := New(cfg)
		req := httptest.NewRequest("GET", "/panel/api/overview", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	// ① 降级中：返回 RFC3339 且可被前端解析
	future := time.Now().Add(90 * time.Minute).Truncate(time.Second)
	got := overview(Config{
		Version: "test", APIKey: "test-key", Pool: pool.New(""),
		DegradedUntil: func() time.Time { return future },
	})
	s, _ := got["degraded_until"].(string)
	if s == "" {
		t.Fatalf("降级中应透出 degraded_until，got %v", got["degraded_until"])
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("degraded_until 不是 RFC3339：%q", s)
	}
	if !parsed.Equal(future) {
		t.Errorf("degraded_until=%v want %v", parsed, future)
	}

	// ② 已过期：即使回调还返回旧时刻，也不能声称"降级中"（否则永远挂着一个假状态）
	past := time.Now().Add(-time.Minute)
	got = overview(Config{
		Version: "test", APIKey: "test-key", Pool: pool.New(""),
		DegradedUntil: func() time.Time { return past },
	})
	if s, _ := got["degraded_until"].(string); s != "" {
		t.Errorf("已过期应为空串，got %q", s)
	}

	// ③ 未接入（nil 回调）：空串，不报错
	got = overview(Config{Version: "test", APIKey: "test-key", Pool: pool.New("")})
	if s, ok := got["degraded_until"].(string); !ok || s != "" {
		t.Errorf("未接入应为空串，got %v", got["degraded_until"])
	}
}
