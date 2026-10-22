package panel

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// 本文件覆盖「上游健康」卡片（P3）的前端纯函数。
//
// 为什么要有：healthStatusLevel / healthWorst 是「黄还是红」的唯一判据，
// healthRowHtml / healthDetailHtml 决定**非绿探针会不会显形**——三者都只在浏览器里
// 生效，Go 侧不执行 JS，编译与后端测试对它们完全失明。判据写反（比如把 yellow 当 green）
// 不会报错，只会让告警静默消失，故在此用 node harness 钉死。
//
// 注意：这里刻意只测**纯函数**（输入数据 → 输出字符串）。DOM 渲染（renderHealth）
// 不进单测——它靠 $() 取节点，mock DOM 的成本远高于收益，而真正的风险在判据与转义上。

const healthJSRegion = `
const fs = require('fs');
const vm = require('vm');
const src = fs.readFileSync(process.argv[2], 'utf8');

function slice(from, to) {
  const a = src.indexOf(from);
  const b = src.indexOf(to);
  if (a < 0 || b < 0 || b <= a) throw new Error('region not found: ' + from + ' .. ' + to);
  return src.slice(a, b);
}
const escSrc = slice('function esc', 'function ago');
const healthSrc = slice('function shortVer', 'let healthCache');

const ctx = { Array, Object, Number, String, JSON };
vm.createContext(ctx);
vm.runInContext(escSrc + '\nthis.esc = esc;', ctx);
vm.runInContext(healthSrc + '\nthis.healthStatusLevel = healthStatusLevel;' +
  'this.healthWorst = healthWorst; this.healthSummaryText = healthSummaryText;' +
  'this.healthNonGreen = healthNonGreen; this.healthRowHtml = healthRowHtml;' +
  'this.healthDetailHtml = healthDetailHtml; this.healthDetailWrapHtml = healthDetailWrapHtml;' +
  'this.shortVer = shortVer; this.degradeLeftText = degradeLeftText;', ctx);

const out = {
  summary: ctx.healthSummaryText({ summary: { accounts: 4, green: 3, yellow: 1, red: 0 } }),
  summaryNil: ctx.healthSummaryText(null),
  lvGreen: ctx.healthStatusLevel('green'),
  lvYellow: ctx.healthStatusLevel('yellow'),
  lvRed: ctx.healthStatusLevel('red'),
  lvUnknown: ctx.healthStatusLevel(undefined),
  worstRed: ctx.healthWorst([{ status: 'green' }, { status: 'red' }, { status: 'yellow' }]),
  worstYellow: ctx.healthWorst([{ status: 'green' }, { status: 'yellow' }]),
  worstNil: ctx.healthWorst(null),
  nonGreen: ctx.healthNonGreen({ probes: [
    { name: 'account', status: 'green' },
    { name: 'tasks', status: 'yellow' },
  ] }).map(function (p) { return p.name; }).join(','),
  rowMixed: ctx.healthRowHtml({ uid: 'abcdef0123456789', realm: 'cn', probes: [
    { name: 'account', status: 'green' },
    { name: 'tasks', status: 'yellow' },
    { name: 'search', status: 'red' },
  ] }),
  rowOk: ctx.healthRowHtml({ uid: 'aaaabbbbcccc', probes: [{ name: 'account', status: 'green' }] }),
  rowEnt: ctx.healthRowHtml({ uid: 'e1', enterprise: true, probes: [] }),
  rowXSS: ctx.healthRowHtml({ uid: '<img src=x onerror=alert(1)>', probes: [] }),
  detail: ctx.healthDetailHtml({ probes: [
    { name: 'buddy', status: 'yellow', latency_ms: 243, detail: '返回 <b>对象</b>', drift: ['buddy.id', 'x'] },
    { name: 'chat', status: 'green', costy: true },
  ] }),
  detailEmpty: ctx.healthDetailHtml({ probes: [] }),
  shortFull: ctx.shortVer('1.13.2-panel+f15ee13'),
  shortPlain: ctx.shortVer('1.13.2-panel'),
  shortDirty: ctx.shortVer('1.13.1-panel+29e5059-dirty'),
  shortEmpty: ctx.shortVer(''),
  wrapHidden: ctx.healthDetailWrapHtml({ probes: [{ name: 'account', status: 'green' }] }, false),
  wrapShown: ctx.healthDetailWrapHtml({ probes: [{ name: 'account', status: 'green' }] }, true),
  dlEmpty: ctx.degradeLeftText('', 0),
  dlInvalid: ctx.degradeLeftText('not-a-date', 0),
  dlExpired: ctx.degradeLeftText('2026-10-21T10:00:00+08:00', Date.parse('2026-10-21T12:00:00+08:00')),
  dl320min: ctx.degradeLeftText('2026-10-21T18:40:00+08:00', Date.parse('2026-10-21T13:20:00+08:00')),
  dl30min: ctx.degradeLeftText('2026-10-21T13:50:00+08:00', Date.parse('2026-10-21T13:20:00+08:00')),
  dl26h: ctx.degradeLeftText('2026-10-22T15:20:00+08:00', Date.parse('2026-10-21T13:20:00+08:00')),
};
process.stdout.write(Object.keys(out).map(function (k) { return k + '=' + String(out[k]); }).join('\n'));
`

func runHealthJSHarness(t *testing.T) map[string]string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; health card frontend test skipped")
	}
	f, err := os.CreateTemp(t.TempDir(), "healthcard-*.cjs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(healthJSRegion); err != nil {
		t.Fatal(err)
	}
	f.Close()

	out, err := exec.Command(node, f.Name(), "app.js").CombinedOutput()
	if err != nil {
		t.Fatalf("health card node harness failed: %v\n%s", err, out)
	}
	// 输出格式是每行 key=value（不用 JSON：这里只需取字段，而中文值与引号转义
	// 会让手工 JSON 解析容易失配，得不偿失）。
	res := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(strings.TrimRight(line, "\r"), "=", 2)
		if len(parts) != 2 {
			continue
		}
		res[parts[0]] = parts[1]
	}
	if len(res) == 0 {
		t.Fatalf("harness 无输出：%s", out)
	}
	return res
}

func TestAppJSHealthStatusLevels(t *testing.T) {
	got := runHealthJSHarness(t)

	want := map[string]string{
		"lvGreen":  "green",
		"lvYellow": "yellow",
		"lvRed":    "red",
		// 未知/缺失一律 yellow：宁可多提示，也不静默当正常（这条最容易写反）
		"lvUnknown":   "yellow",
		"worstRed":    "red",
		"worstYellow": "yellow",
		"worstNil":    "green",
		"nonGreen":    "tasks",
		"summary":     "绿 3 / 黄 1 / 红 0 · 4 个账号",
		"summaryNil":  "绿 0 / 黄 0 / 红 0 · 0 个账号",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestAppJSHealthRowRendering(t *testing.T) {
	got := runHealthJSHarness(t)

	// 混合态：非绿探针逐个显形（带对应染色），绿的不占行内空间
	row := got["rowMixed"]
	for _, frag := range []string{"tasks", "search", "abcdef01", `data-hlt-uid="abcdef0123456789"`} {
		if !strings.Contains(row, frag) {
			t.Errorf("rowMixed 缺少 %q：%s", frag, row)
		}
	}
	if strings.Contains(row, ">account<") {
		t.Errorf("绿探针不该出现在行内：%s", row)
	}
	if !strings.Contains(row, "tag bad") || !strings.Contains(row, "tag warn") {
		t.Errorf("黄/红应采用不同染色：%s", row)
	}
	if !strings.Contains(row, ">red<") {
		t.Errorf("行首应显示最差状态 red：%s", row)
	}

	// 全绿：一行「全部正常」
	if ok := got["rowOk"]; !strings.Contains(ok, "全部正常") {
		t.Errorf("全绿应收敛为「全部正常」：%s", ok)
	}

	// 企业版标记
	if ent := got["rowEnt"]; !strings.Contains(ent, "企业版") {
		t.Errorf("企业版账号应有标记：%s", ent)
	}
}

// 上游返回的 detail / uid 都是外部可控字符串，必须转义——否则一个「模型名」里带标签
// 就能在面板里执行脚本。
func TestAppJSHealthEscaping(t *testing.T) {
	got := runHealthJSHarness(t)

	xss := got["rowXSS"]
	if strings.Contains(xss, "<img") {
		t.Fatalf("uid 未转义，存在注入：%s", xss)
	}
	if !strings.Contains(xss, "&lt;img") {
		t.Errorf("uid 应被转义为实体：%s", xss)
	}

	det := got["detail"]
	if strings.Contains(det, "<b>对象</b>") {
		t.Fatalf("detail 未转义，存在注入：%s", det)
	}
	for _, frag := range []string{"243ms", "&lt;b&gt;", "漂移项：buddy.id, x", "花积分"} {
		if !strings.Contains(det, frag) {
			t.Errorf("detail 缺少 %q：%s", frag, det)
		}
	}

	if empty := got["detailEmpty"]; !strings.Contains(empty, "没有探针结果") {
		t.Errorf("空探针集应有明确提示：%s", empty)
	}
}

// 侧栏版本串：窄栏里必须紧凑，否则换行撑破布局（线上就是这么坏的）。
func TestAppJSShortVersion(t *testing.T) {
	got := runHealthJSHarness(t)
	want := map[string]string{
		"shortFull":  "1.13.2+f15ee13",       // 去掉 -panel，commit 保留 7 位
		"shortPlain": "1.13.2",               // 无 commit 时只剩版本
		"shortDirty": "1.13.1+29e5059-dirty", // dirty 标记必须保留
		"shortEmpty": "",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// 明细展开状态：轮询会重建 DOM，hidden 必须由「展开集合」驱动，不能渲染时写死。
func TestAppJSHealthDetailWrap(t *testing.T) {
	got := runHealthJSHarness(t)

	if h := got["wrapHidden"]; !strings.Contains(h, " hidden") {
		t.Errorf("未展开时应带 hidden 属性：%s", h)
	}
	if s := got["wrapShown"]; strings.Contains(s, "hidden") {
		t.Errorf("已展开时不应带 hidden 属性：%s", s)
	}
	if s := got["wrapShown"]; !strings.Contains(s, "account") {
		t.Errorf("已展开的容器应含探针明细：%s", s)
	}
}

// 降级剩余时长文案：写反了不会报错，只会让用户永远看不到时限。
func TestAppJSDegradeLeftText(t *testing.T) {
	got := runHealthJSHarness(t)
	want := map[string]string{
		"dlEmpty":   "",             // 未降级
		"dlInvalid": "",             // 脏数据不能崩，也不能瞎显示
		"dlExpired": "",             // 已过期不能显示"还剩 -120 分"
		"dl320min":  "还剩 5 小时 20 分", // 5h20m
		"dl30min":   "还剩 30 分",      // 不足 1 小时
		"dl26h":     "还剩 1 天 2 小时",  // 跨天用"天/小时"，不显示分钟
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}
