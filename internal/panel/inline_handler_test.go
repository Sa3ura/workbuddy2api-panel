package panel

import (
	"os"
	"regexp"
	"testing"
)

// TestIndexHTMLHasNoInlineEventHandlers CSP 是 `script-src 'self'`，**没有**
// unsafe-inline / unsafe-hashes，因此 HTML 里的 `on*="..."` 内联事件处理器会被
// 浏览器直接拦下，控制台报：
//
//	Executing inline event handler violates the following Content Security
//	Policy directive 'script-src 'self''...
//
// 而且被拦掉之后**默认行为照常发生**：`<form onsubmit="return false">` 里的回车
// 会真实提交、页面刷新、密钥弹窗状态丢失——所以它不是"只是报个错"，是功能性的坑
// （2026-10-22 线上实案：填完 api_key 回车，key 没生效还看不出为什么）。
//
// 阻止提交/点击的职责一律放 app.js 的 addEventListener 里。
func TestIndexHTMLHasNoInlineEventHandlers(t *testing.T) {
	b, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	// 只匹配「空白 + on + 至少 3 个字母 + =」，避免把 data-* / 正文里的
	// "on..." 误判成属性。
	re := regexp.MustCompile(`(?i)\son[a-z]{3,}\s*=`)
	if m := re.FindIndex(b); m != nil {
		start := m[0] - 60
		if start < 0 {
			start = 0
		}
		end := m[1] + 60
		if end > len(b) {
			end = len(b)
		}
		t.Fatalf("index.html 出现内联事件处理器（会被 CSP 拦掉，且默认行为照常发生）：\n  %q\n"+
			"改为在 app.js 里 addEventListener 绑定。", b[start:end])
	}
}
