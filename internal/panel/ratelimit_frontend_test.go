package panel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 本文件钉死「模型锁行」的前端纯函数 rateLimitMeta（模型锁池 / 账号行下方的
// 限流明细）。它只在账号带 rate_limited_models 时才会被调用，所以这类缺陷
// 平时完全不显形。
//
// 2026-10-22 线上实案：该函数两个分支写成 `detail = ...` 而从未声明 detail，
// 而 app.js 首行是 'use strict' —— 未声明赋值抛 ReferenceError，把整个
// renderAccounts 干掉，表现为「账号池 / 模型锁池两张表全空」，而 loadOverview
// 的 catch 在 quiet=true 时又是静默的，前端空白无从排查。
//
// 所以这里的 vm 上下文**必须**加 'use strict' 前缀：不加就退化成 sloppy mode，
// `detail = ...` 会静默创建全局变量，测试全绿而 bug 照旧——等于没测。

const rateLimitMetaHarness = `'use strict';
const fs = require('fs'), vm = require('vm');
const src = fs.readFileSync(process.argv[2], 'utf8');
const a = src.indexOf('function rateLimitMeta');
const b = src.indexOf('function rateLimitRowsHtml');
if (a < 0 || b < 0 || b < a) throw new Error('rateLimitMeta region not found');
const ctx = {
  Array, Object, String, Number, Math, Date,
  parseAPITime: s => { const d = new Date(s); return isNaN(d) ? null : d; },
  fmtLocalDateTime: d => { const x = new Date(d); const p = n => String(n).padStart(2, '0');
    return x.getFullYear() + '-' + p(x.getMonth() + 1) + '-' + p(x.getDate()) + ' ' +
      p(x.getHours()) + ':' + p(x.getMinutes()) + ':' + p(x.getSeconds()); },
  dur: n => n + 's',
};
vm.createContext(ctx);
vm.runInContext("'use strict';" + src.slice(a, b) + ';this.meta = rateLimitMeta;', ctx);

const now = Date.parse('2026-10-22 20:00:00');   // 与 parseAPITime 同一时区基准
const dual = ctx.meta({ model: 'deepseek-v4-flash', kind: 'rate_limit',
  reset_at: '2026-10-22 22:49:09', until: '2026-10-22 21:00:00' }, now);
const single = ctx.meta({ model: 'm', kind: 'rate_limit', reset_at: '2026-10-22 22:49:09' }, now);
const noTime = ctx.meta({ model: 'm', kind: 'rate_limit' }, now);
const unavail = ctx.meta({ model: 'm', kind: 'model_unavailable', until: '2026-10-22 21:00:00' }, now);

process.stdout.write(JSON.stringify({
  dualDetail: typeof dual.detail === 'string' && dual.detail.length > 0,
  singleDetail: typeof single.detail === 'string' && single.detail.length > 0,
  noTimeDetail: typeof noTime.detail === 'string' && noTime.detail.length > 0,
  unavailDetail: typeof unavail.detail === 'string' && unavail.detail.length > 0,
  dualMentionsBothClocks:
    dual.detail.includes('21:00:00') && dual.detail.includes('22:49:09'),
  modelKept: dual.model === 'deepseek-v4-flash' && unavail.kind === 'model_unavailable',
}));
`

func runRateLimitMetaHarness(t *testing.T, appJS string) (string, error) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; rateLimitMeta harness skipped")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "ratelimit-meta.cjs")
	if err := os.WriteFile(script, []byte(rateLimitMetaHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, appJS).CombinedOutput()
	return string(out), err
}

// TestAppJSRateLimitMetaStrictMode 四个分支都必须返回字符串 detail。
// 缺 detail 声明时（历史 bug）这里会以 ReferenceError 失败。
func TestAppJSRateLimitMetaStrictMode(t *testing.T) {
	out, err := runRateLimitMetaHarness(t, "app.js")
	if err != nil {
		t.Fatalf("rateLimitMeta harness failed（'use strict' 下抛错 = detail 未声明）: %v\n%s", err, out)
	}
	want := `{"dualDetail":true,"singleDetail":true,"noTimeDetail":true,"unavailDetail":true,` +
		`"dualMentionsBothClocks":true,"modelKept":true}`
	if strings.TrimSpace(out) != want {
		t.Fatalf("rateLimitMeta = %s\nwant %s", strings.TrimSpace(out), want)
	}
}

// TestAppJSRateLimitMetaMutationDiscriminates 反向证明：把 `let detail;` 删掉，
// 本用例必须 FAIL。没有这条，「正向全绿」可能只是因为 vm 跑在 sloppy mode。
func TestAppJSRateLimitMetaMutationDiscriminates(t *testing.T) {
	orig, err := os.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	const decl = "let detail;"
	if !strings.Contains(string(orig), decl) {
		t.Fatalf("变异未生效：app.js 里找不到 %q（修复被改写？）", decl)
	}
	mut := filepath.Join(t.TempDir(), "app-mut.js")
	if err := os.WriteFile(mut, []byte(strings.Replace(string(orig), decl, "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runRateLimitMetaHarness(t, mut); err == nil {
		t.Fatal("删掉 `let detail;` 之后 harness 仍然通过 —— 判据没有判别力")
	}
}
