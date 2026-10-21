package upstream

// probe_test.go 探针判定逻辑的契约测试。
//
// 用 httptest 造假上游，只验证**判定**（绿/黄/红 + 企业版豁免），不验证上游行为。
// 这样上游真改了协议时，本文件不会红——它保护的是"探针本身别失灵"，
// 而"上游是否变了"由 cmd/doctor 的真实运行回答（两件事必须分开，否则探针会
// 把当时的 bug 固化成"正确形状"）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
)

// fakeUpstream 返回一个指向本地的 client：把三个域都指向同一个 fake server。
func fakeUpstream(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New()
	c.ChatBaseCN = srv.URL
	c.BillingBaseCN = srv.URL
	c.WebBaseCN = srv.URL
	c.ChatBaseGlobal = srv.URL
	c.BillingBaseGlobal = srv.URL
	return c
}

func probeTestAuth(uid string, enterprise bool) *auth.Auth {
	a := &auth.Auth{
		UID:         uid,
		AccessToken: "test-token",
		Domain:      "codebuddy.cn",
	}
	if enterprise {
		a.EnterpriseID = "ent-123"
	}
	return a
}

// okEnvelope 返回业务成功的信封（doJSON 要求 code==0）。
func okEnvelope(data string) string {
	return `{"code":0,"msg":"ok","data":` + data + `}`
}

// 1) 任务列表命中熟悉的老任务码 → 绿。
func TestProbeTasks_Green(t *testing.T) {
	c := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/activity/growth/tasks") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(okEnvelope(`{"tasks":[
			{"task_code":"first_buddy"},{"task_code":"chat_5"},{"task_code":"template_5"},
			{"task_code":"skill_1"},{"task_code":"Buddy_App"},{"task_code":"create_canvas"},
			{"task_code":"automation_1"},{"task_code":"Library_read"},
			{"task_code":"playbook_prompt"},{"task_code":"Hp_Appearance"},
			{"task_code":"some_new_task_from_upstream"}
		]}`)))
	})

	out := probeTasks(context.Background(), c, probeTestAuth("u1", false))
	if out.status != ProbeGreen {
		t.Fatalf("want green, got %s (%s)", out.status, out.detail)
	}
}

// 2) 老任务码集体消失（上游改口径）→ 黄，且漂移项能列出来。
func TestProbeTasks_YellowOnDrift(t *testing.T) {
	c := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(okEnvelope(`{"tasks":[{"task_code":"totally_new_a"},{"task_code":"totally_new_b"}]}`)))
	})

	out := probeTasks(context.Background(), c, probeTestAuth("u1", false))
	if out.status != ProbeYellow {
		t.Fatalf("want yellow, got %s (%s)", out.status, out.detail)
	}
	if len(out.drift) == 0 {
		t.Fatal("漂移时应列出缺失的任务码")
	}
}

// 3) 任务列表为空 → 黄（不是绿：空列表几乎必然是口径变了）。
func TestProbeTasks_YellowOnEmpty(t *testing.T) {
	c := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(okEnvelope(`{"tasks":[]}`)))
	})

	out := probeTasks(context.Background(), c, probeTestAuth("u1", false))
	if out.status != ProbeYellow {
		t.Fatalf("want yellow, got %s (%s)", out.status, out.detail)
	}
}

// 4) 企业版访问个人成长端点 403 → 判绿（预期行为，不能假告警）。
func TestProbe_Enterprise403IsGreen(t *testing.T) {
	c := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":10001,"msg":"growth system is only available for personal users"}`))
	})

	a := probeTestAuth("ent", true)
	if out := probeStreak(context.Background(), c, a); out.status != ProbeGreen {
		t.Fatalf("streak: 企业版 403 应判绿，got %s (%s)", out.status, out.detail)
	}
	if out := probeTasks(context.Background(), c, a); out.status != ProbeGreen {
		t.Fatalf("tasks: 企业版 403 应判绿，got %s (%s)", out.status, out.detail)
	}
}

// 5) 个人号遇到 403 → 红（这才是真的坏了）。
func TestProbe_Personal403IsRed(t *testing.T) {
	c := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":10001,"msg":"growth system is only available for personal users"}`))
	})

	out := probeStreak(context.Background(), c, probeTestAuth("p1", false))
	if out.status != ProbeRed {
		t.Fatalf("个人号 403 应判红，got %s (%s)", out.status, out.detail)
	}
}

// 6) 5xx → 红。
func TestProbe_ServerErrorIsRed(t *testing.T) {
	c := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	})

	out := probeStreak(context.Background(), c, probeTestAuth("p1", false))
	if out.status != ProbeRed {
		t.Fatalf("5xx 应判红，got %s (%s)", out.status, out.detail)
	}
}

// 7) 业务 code != 0 → 红（HTTP 200 也不算通）。
func TestProbe_BusinessCodeNonZeroIsRed(t *testing.T) {
	c := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":50001,"msg":"参数错误"}`))
	})

	out := probeStreak(context.Background(), c, probeTestAuth("p1", false))
	if out.status != ProbeRed {
		t.Fatalf("code!=0 应判红，got %s (%s)", out.status, out.detail)
	}
}

// 8) 汇总与退出码映射：红 → 2，黄 → 1，全绿 → 0。
func TestHealthReport_ExitCode(t *testing.T) {
	cases := []struct {
		name string
		rep  *HealthReport
		want int
	}{
		{"nil", nil, 2},
		{"green", &HealthReport{Summary: HealthSummary{Accounts: 1, Green: 1}}, 0},
		{"yellow", &HealthReport{Summary: HealthSummary{Accounts: 1, Yellow: 1}}, 1},
		{"red", &HealthReport{Summary: HealthSummary{Accounts: 2, Red: 1, Green: 1}}, 2},
		{"red+empty", &HealthReport{}, 0},
	}
	for _, tc := range cases {
		if got := tc.rep.ExitCode(); got != tc.want {
			t.Errorf("%s: want %d, got %d", tc.name, tc.want, got)
		}
	}
}

// 9) 集合交集：缺失项与比例。
func TestIntersectionRatio(t *testing.T) {
	missing, ratio := intersectionRatio([]string{"a", "b", "c", "d"}, []string{"A", "c"})
	if ratio != 0.5 {
		t.Fatalf("ratio want 0.5, got %v", ratio)
	}
	if strings.Join(missing, ",") != "b,d" {
		t.Fatalf("missing want b,d got %v", missing)
	}

	// want 为空时视为全命中（不因缺少基准而误报）
	if _, r := intersectionRatio(nil, []string{"x"}); r != 1 {
		t.Fatalf("空基准应返回 1，got %v", r)
	}
}

// 10) 探针集必须是只读的：默认集里不得出现花积分的探针。
func TestDefaultProbes_AreFree(t *testing.T) {
	for _, p := range defaultProbes() {
		if p.Costy {
			t.Errorf("默认探针 %s 标记为花积分，违反「默认全部免费只读」约束", p.Name)
		}
	}
	if !chatProbe().Costy {
		t.Error("chat 探针必须标记 Costy=true")
	}
}

// 11) 单个探针结果的状态聚合：账号状态取最差。
func TestRunHealthProbes_WorstStatusWins(t *testing.T) {
	c := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/activity/growth/tasks"):
			// 漂移 → 黄
			_, _ = w.Write([]byte(okEnvelope(`{"tasks":[{"task_code":"unknown_only"}]}`)))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
		}
	})

	h := RunHealthProbes(context.Background(), c, probeTestAuth("agg", false), false)
	if h.Status != ProbeRed {
		t.Fatalf("有红探针时账号状态应为 red，got %s", h.Status)
	}
	var sawYellow bool
	for _, p := range h.Probes {
		if p.Name == "tasks" && p.Status == ProbeYellow {
			sawYellow = true
		}
	}
	if !sawYellow {
		t.Error("tasks 探针应为 yellow")
	}
}
