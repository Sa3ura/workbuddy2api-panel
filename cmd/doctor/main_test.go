// main_test.go cmd/doctor 的取证导出必须不泄漏凭证。
//
// 为什么值得单独测：`-headers` 的输出会被贴进 docs/protocol/ 入库。一旦遮蔽漏了
// 某个头，就等于把 accessToken / refreshToken / 设备指纹写进版本库——这类事故不可回滚
// （git 历史清不掉）。所以遮蔽逻辑必须有测试兜住，而不是靠肉眼检查。
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

// 假凭证：故意用可识别的长串，便于断言"原值不得出现在输出里"。
const (
	fakeAccess  = "eyJhbGciOiJSUzI1NiJ9.fake-access-token-value-0123456789"
	fakeRefresh = "eyJhbGciOiJSUzI1NiJ9.fake-refresh-token-value-0123456789"
	fakeUID     = "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0"
)

func testAccount() *auth.Auth {
	return &auth.Auth{
		UID:          fakeUID,
		AccessToken:  fakeAccess,
		RefreshToken: fakeRefresh,
		Domain:       "codebuddy.cn",
	}
}

// 所有敏感头的原值都必须被遮蔽；非敏感头（UA / Origin / 版本）必须原样保留。
func TestDumpHeaders_MasksSecrets(t *testing.T) {
	dumps := dumpHeaders(upstream.New(), []*auth.Auth{testAccount()})
	if len(dumps) == 0 {
		t.Fatal("没有导出任何头清单")
	}

	raw, err := json.Marshal(dumps)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)

	for _, secret := range []string{fakeAccess, fakeRefresh, fakeUID} {
		if strings.Contains(out, secret) {
			t.Fatalf("凭证/账号标识泄漏进导出：%s", secret)
		}
	}

	// 派生标识（uid 的 sha256 前 36 hex）也不该原样出现——它等价账号指纹。
	for _, d := range dumps {
		for k, v := range d.Headers {
			canon := strings.ToLower(k)
			if canon == "x-machine-id" || canon == "x-session-id" {
				if !strings.Contains(v, "redacted") {
					t.Errorf("%s 未遮蔽：%q", k, v)
				}
			}
		}
	}
}

// 头族完整性：四类请求各自的标志性头必须在场（少一个就是头族退化，风控会先炸这里）。
func TestDumpHeaders_HeaderFamilies(t *testing.T) {
	dumps := dumpHeaders(upstream.New(), []*auth.Auth{testAccount()})
	byKind := make(map[string]map[string]string, len(dumps))
	for _, d := range dumps {
		byKind[d.Kind] = d.Headers
	}

	require := func(kind string, headers ...string) {
		h, ok := byKind[kind]
		if !ok {
			t.Fatalf("缺少头族：%s", kind)
		}
		for _, want := range headers {
			if _, ok := h[want]; !ok {
				t.Errorf("%s 缺少头 %s", kind, want)
			}
		}
	}

	require("chat（对话 / 流式）",
		"Authorization", "X-Codebuddy-Request", "X-Machine-Id", "X-Session-Id",
		"X-Agent-Purpose", "X-Ide-Name", "X-Ide-Version", "X-Product", "X-Domain")
	require("billing（签到 / 余额 / 补偿）",
		"Authorization", "X-Codebuddy-Request", "X-User-Id")
	require("refresh（token 刷新）",
		"X-Refresh-Token", "X-Auth-Refresh-Source", "X-Machine-Id")

	// 安全红线：refreshToken 只出现在刷新请求，绝不能出现在 chat / billing 里。
	for kind, h := range byKind {
		if kind == "refresh（token 刷新）" {
			continue
		}
		if _, ok := h["X-Refresh-Token"]; ok {
			t.Errorf("%s 不得携带 X-Refresh-Token（安全红线）", kind)
		}
	}
}
