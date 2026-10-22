package upstream

import (
	"net/http"
	"testing"
)

// 官方「设备身份三件套」里可复制部分的行为锁定。
//
// 证据来源（A 级，客户端源码）：D:\WorkBuddy\resources\app.asar
//
//	/main/module.app-server.js：
//	  var x=`X-Device-Token`, S=`X-Custom-Channel-Id`, ee=`X-Device-Id-List`, te=`unknown`
//	  C = class { async provide() {
//	    let e = { [S]: te }                                  // ① 恒为字面量 unknown
//	    try { let t = await this.authService.getTuringDeviceToken(); t && (e[x] = t) }  // ② Turing 令牌
//	    catch { logger.warn('[auth/state] resolve device token failed, skip') }
//	    let t = process.env.CODEBUDDY_QIMEI36?.trim()
//	    t && (e[ee] = JSON.stringify({ qimei36: t }))         // ③ QIMEI 指纹
//	    return e } }
//	官方头构造器（同 asar）：
//	  t['X-Private-Data'] = 'true'                            // 无条件，与 X-Product 同批
//
// ①③ 里只有 ① 可以完全照抄（常量）；②③ 依赖腾讯原生 SDK（TuringShieldSDK.dll /
// @tencent/qimei-node），离线无法合成 —— 见 docs/protocol/headers.md「设备身份三件套」。
//
// 这两个头写反或漏发不会报错，只会让出站请求比官方少一项身份，属于静默降级，故用测试钉住。
func TestDeviceIdentityHeaders(t *testing.T) {
	c := New()
	a := testAuth()

	// CommonHeaders（所有 API 请求）：X-Custom-Channel-Id
	req, err := http.NewRequest(http.MethodPost, "https://www.codebuddy.cn/v2/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.CommonHeaders(req, a)
	if got := req.Header.Get("X-Custom-Channel-Id"); got != "unknown" {
		t.Errorf("CommonHeaders X-Custom-Channel-Id = %q, want %q（官方分发的字面量值）", got, "unknown")
	}
	if got := req.Header.Get("X-Machine-ID"); got == "" {
		t.Errorf("CommonHeaders 应仍带 X-Machine-ID（历史行为，未在本轮移除）")
	}

	// ChatHeaders（对话路径）：X-Private-Data + 继承 X-Custom-Channel-Id
	req2, err := http.NewRequest(http.MethodPost, "https://www.codebuddy.cn/v2/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.ChatHeaders(req2, a, "", ChatMeta{})
	if got := req2.Header.Get("X-Private-Data"); got != "true" {
		t.Errorf("ChatHeaders X-Private-Data = %q, want %q", got, "true")
	}
	if got := req2.Header.Get("X-Custom-Channel-Id"); got != "unknown" {
		t.Errorf("chat 应继承 X-Custom-Channel-Id，got %q", got)
	}
	if got := req2.Header.Get("X-IDE-Version"); got != defaultClientVersion {
		t.Errorf("chat X-IDE-Version = %q, want %q（须与 UA 版本段一致）", got, defaultClientVersion)
	}
}
