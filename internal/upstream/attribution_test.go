package upstream

import (
	"net/http"
	"testing"
)

// TestInjectAttributionProductDecoupledFromClientName 1.13.10 的核心契约：
// **X-Product 与 client_name（X-IDE-* 的名字）是两个独立维度**。
//
// 拆分前 `client_name` 一个键同时决定 X-Product 与 X-IDE-* 四头，且 `client_name="SaaS"`
// 会把 X-IDE-* 整组砍掉 —— 于是"只想把 X-Product 改成官方默认的 SaaS"做不到。
// 拆分后必须能独立配、且旧写法语义不变。
func TestInjectAttributionProductDecoupledFromClientName(t *testing.T) {
	for _, tc := range []struct {
		name        string
		clientName  string
		product     string
		wantProduct string
		wantIDE     bool
	}{
		{"默认（两者都不配）", "", "", "SaaS", true},
		{"product=WorkBuddy 回退 1.13.9 取值", "", "WorkBuddy", "WorkBuddy", true},
		{"client_name 改名只影响 IDE 四头", "XB", "", "SaaS", true},
		{"client_name=SaaS 旧语义：不发 IDE 四头", "SaaS", "", "SaaS", false},
		{"product 优先于 client_name 的 SaaS 语义", "SaaS", "Custom", "Custom", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{ClientName: tc.clientName, Product: tc.product}
			req, err := http.NewRequest(http.MethodPost, "https://chat.example/v2/chat/completions", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			c.injectAttribution(req)

			if got := req.Header.Get("X-Product"); got != tc.wantProduct {
				t.Errorf("X-Product = %q want %q", got, tc.wantProduct)
			}
			if got := req.Header.Get("X-IDE-Name"); (got != "") != tc.wantIDE {
				t.Errorf("X-IDE-Name = %q, wantIDE = %v", got, tc.wantIDE)
			}
			// 官方桌面端头构造器无条件设置该头（即使 X-Product=SaaS 也照发）。
			if got := req.Header.Get("X-Private-Data"); got != "true" {
				t.Errorf("X-Private-Data = %q want true", got)
			}
			if !tc.wantIDE {
				return
			}
			if got := req.Header.Get("X-IDE-Type"); got != tc.clientName && tc.clientName != "" {
				t.Errorf("X-IDE-Type = %q want %q", got, tc.clientName)
			}
			// X-IDE-Version 必须跟随 client_version（默认常量），而不是写死的另一套值。
			if got := req.Header.Get("X-IDE-Version"); got != defaultClientVersion {
				t.Errorf("X-IDE-Version = %q want %q（= defaultClientVersion）", got, defaultClientVersion)
			}
			if got := req.Header.Get("X-Agent-Purpose"); got != "conversation" {
				t.Errorf("X-Agent-Purpose = %q want conversation", got)
			}
		})
	}
}

// TestProductNameDefaultIsSaaS X-Product 默认值与官方两客户端一致：
// 桌面头构造器 `deploymentType?.trim() || "SaaS"`、CLI interceptor `… ?? "SaaS"`。
// 回归护栏：把默认改回 WorkBuddy 会让这条红。
func TestProductNameDefaultIsSaaS(t *testing.T) {
	if got := (&Client{}).productName(); got != "SaaS" {
		t.Errorf("productName() = %q want SaaS（官方桌面端与 CLI 缺省值）", got)
	}
	if got := (*Client)(nil).productName(); got != "SaaS" {
		t.Errorf("nil client productName() = %q want SaaS（空值安全）", got)
	}
	if got := (&Client{Product: "WorkBuddy"}).productName(); got != "WorkBuddy" {
		t.Errorf("显式 Product 未生效：%q", got)
	}
}

// TestDefaultVersionsFollowBundledClients 版本段默认值只做**形状**约束，不钉具体数字
// （版本值单点维护在 headers.go，随桌面端更新而变；钉数字会腐烂三轮了）。
func TestDefaultVersionsFollowBundledClients(t *testing.T) {
	if got := (&Client{}).clientVersion(); got != defaultClientVersion || got == "" {
		t.Errorf("clientVersion() = %q want %q", got, defaultClientVersion)
	}
	if got := (&Client{}).cliVersion(); got != defaultCliVersion || got == "" {
		t.Errorf("cliVersion() = %q want %q", got, defaultCliVersion)
	}
	// 显式配置必须覆盖内置默认。
	c := &Client{ClientVersion: "9.9.9", CliVersion: "8.8.8"}
	if got := c.clientVersion(); got != "9.9.9" {
		t.Errorf("clientVersion() = %q want 9.9.9（配置覆盖）", got)
	}
	if got := c.cliVersion(); got != "8.8.8" {
		t.Errorf("cliVersion() = %q want 8.8.8（配置覆盖）", got)
	}
	// UA 三段式形状：两段 WorkBuddy + 一段 CLI。
	want := "WorkBuddy/9.9.9 WorkBuddy/9.9.9 CLI/8.8.8"
	if got := c.defaultWorkBuddyUA(); got != want {
		t.Errorf("defaultWorkBuddyUA() = %q want %q", got, want)
	}
}
