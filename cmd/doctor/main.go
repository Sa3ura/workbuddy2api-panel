// Command doctor 上游协议漂移探针（只读）。
//
// 用途：定期检查网关依赖的上游端点是否仍然"长成预期样子"，把「上游悄悄改协议 →
// 用户报错 → 现场逆向」反转成「探针告警 → 按需修」。设计说明见 EXEC-PLAN.md P3、
// 判定规则见 internal/upstream/probe.go 文件头。
//
// 只读保证：默认探针集不含任何写操作（不签到 / 不领奖 / 不上报 / 不建会话 / 不刷新
// token），可随时随地被任何人跑，不消耗积分、不改变账号状态。唯一花积分的是
// `-chat`（显式开启才跑）。
//
// 用法：
//
//	go run ./cmd/doctor -config config.json           # 人类可读表
//	go run ./cmd/doctor -config config.json -json     # 机读
//	go run ./cmd/doctor -account <uid>                # 只查一个号
//	go run ./cmd/doctor -chat                         # 追加对话探针（花积分）
//
// 退出码：0 全绿 / 1 有黄 / 2 有红（便于宿主 cron、告警接入）。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/logfmt"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

// doctorConfig 是 config.json 中 doctor 关心的最小子集。
//
// 为什么在这里重复定义而不复用 cmd/server/config.go：那是 package main，无法被导入。
// 为诊断工具把整个 server 配置层拆成独立包，收益不抵风险（会牵动装配期字段与
// 30k 行配置测试）。这里只读、不写配置，字段缺失一律回落默认值——**不与 server 争
// 装配权**，只对齐影响到「探针发出什么样请求」的那几个字段（UA / 版本 / 超时 / 域）。
type doctorConfig struct {
	AuthDir string `json:"auth_dir"`
	Global  struct {
		Enabled     *bool  `json:"enabled"`
		ChatBase    string `json:"chat_base"`
		BillingBase string `json:"billing_base"`
	} `json:"global"`
	CN struct {
		ChatBase string `json:"chat_base"`
	} `json:"cn"`
	Upstream struct {
		TimeoutSeconds int    `json:"timeout_seconds"`
		UserAgent      string `json:"user_agent"`
		ClientVersion  string `json:"client_version"`
		CliVersion     string `json:"cli_version"`
	} `json:"upstream"`
}

func main() {
	var (
		cfgPath  = flag.String("config", "config.json", "配置文件路径（缺失时用默认值）")
		authDir  = flag.String("auth-dir", "", "凭证目录（默认取配置的 auth_dir，再回落 ./auths）")
		uid      = flag.String("account", "", "只检查指定 uid")
		asJSON   = flag.Bool("json", false, "输出 JSON（默认人类可读表）")
		outPath  = flag.String("out", "data/upstream_health.json", "健康报告落盘路径；none/off/- 表示不落盘")
		withChat = flag.Bool("chat", false, "追加对话探针（**消耗积分**，默认不跑）")
		withKeys = flag.Bool("keys", false, "只导出各 growth 端点的响应键位骨架（键名，无值；用于重建协议档案）")
		withMods = flag.Bool("models", false, "导出模型目录实测字段（id/上下文/输出上限/档位/倍率；用于重建 model-catalog 档案）")
		timeout  = flag.Duration("timeout", 90*time.Second, "整体超时")
	)
	flag.Parse()

	cfg := loadConfig(*cfgPath)
	dir := *authDir
	if dir == "" {
		dir = cfg.AuthDir
	}
	if dir == "" {
		dir = "./auths"
	}

	accounts, err := auth.LoadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取凭证目录失败 %s: %v\n", dir, err)
		os.Exit(2)
	}
	if len(accounts) == 0 {
		fmt.Fprintf(os.Stderr, "凭证目录 %s 里没有账号（auths/workbuddy-<uid>.json）\n", dir)
		os.Exit(2)
	}
	if *uid != "" {
		filtered := accounts[:0]
		for _, a := range accounts {
			if a != nil && a.UID == *uid {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintf(os.Stderr, "找不到 uid=%s\n", *uid)
			os.Exit(2)
		}
		accounts = filtered
	}

	client := buildClient(cfg, *timeout)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// -keys：只导出键位骨架，不跑探针（用于 P1 重建协议档案时核对字段名）
	if *withKeys {
		var all []upstream.KeySkeleton
		for _, a := range accounts {
			all = append(all, upstream.DumpGrowthKeys(ctx, client, a)...)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(all)
		return
	}

	// -models：导出模型目录实测字段（重建 model-catalog / thinking-effort 档案用）
	if *withMods {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(dumpModels(ctx, client, accounts))
		return
	}

	rep := upstream.RunHealthReport(ctx, client, accounts, *withChat, appVersion)

	if !reportDisabled(*outPath) {
		writeReport(*outPath, rep)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printTable(rep)
	}

	os.Exit(rep.ExitCode())
}

// appVersion 与 server 版本解耦：探针报告的版本号用于对照「哪版网关依赖了这套协议」。
const appVersion = "doctor-1"

// modelDump 模型目录的取证条目（重建 model-catalog / thinking-effort 档案用）。
//
// 只含**模型自身属性**：不含 uid、不含任何账号数据。模型目录本身是公开信息，
// 因此这份导出可以安全入库到 docs/protocol/ 作为 A 级证据。
type modelDump struct {
	Realm           string   `json:"realm"`
	ID              string   `json:"id"`
	Name            string   `json:"name,omitempty"`
	ContextWindow   int64    `json:"context_window,omitempty"`
	MaxTokens       int64    `json:"max_tokens,omitempty"`
	MaxAllowedSize  int64    `json:"max_allowed_size,omitempty"`
	Efforts         []string `json:"efforts,omitempty"`
	DefaultEffort   string   `json:"default_effort,omitempty"`
	CanDisableThink bool     `json:"can_disable_thinking,omitempty"`
	Credits         string   `json:"credits,omitempty"`
	PromoCredits    string   `json:"promo_credits,omitempty"`
	PromoLabel      string   `json:"promo_label,omitempty"`
	SupportsTools   bool     `json:"supports_tool_call,omitempty"`
	SupportsImages  bool     `json:"supports_images,omitempty"`
	Vendor          string   `json:"vendor,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

// dumpModels 拉取每个账号视角的模型目录并压成取证条目。
// 失败只打印警告（目录拉取可能因账号/域不同而失败），不影响其它账号。
func dumpModels(ctx context.Context, c *upstream.Client, accounts []*auth.Auth) []modelDump {
	out := make([]modelDump, 0, 32)
	for _, a := range accounts {
		infos, err := c.FetchModels(a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告：uid=%s 模型目录拉取失败：%v\n", logfmt.UID8(a.UID), err)
			continue
		}
		for _, m := range infos {
			out = append(out, modelDump{
				Realm:           a.Realm(),
				ID:              m.ID,
				Name:            m.Name,
				ContextWindow:   m.ContextWindow,
				MaxTokens:       m.MaxTokens,
				MaxAllowedSize:  m.MaxAllowedSize,
				Efforts:         m.Efforts,
				DefaultEffort:   m.DefaultEffort,
				CanDisableThink: m.CanDisableThinking,
				Credits:         m.Credits,
				PromoCredits:    m.PromoCredits,
				PromoLabel:      m.PromoLabel,
				SupportsTools:   m.SupportsToolCall,
				SupportsImages:  m.SupportsImages,
				Vendor:          m.Vendor,
				Tags:            m.Tags,
			})
		}
	}
	return out
}

// reportDisabled 判断 -out 是否要求不落盘。
//
// 为什么不用「空字符串」表示关闭：**Windows PowerShell 5.1 会把空字符串参数直接丢掉**，
// `-out ”  -timeout 90s` 到 Go 这边变成 `-out -timeout 90s`——于是报告被写进一个
// 名为 `-timeout` 的文件（本项目实际踩过）。所以关闭写盘要显式写 none/off/-。
func reportDisabled(path string) bool {
	switch strings.ToLower(strings.TrimSpace(path)) {
	case "", "none", "off", "-", "null":
		return true
	default:
		return false
	}
}

// loadConfig 读配置；文件不存在或解析失败**不致命**（探针仍可用默认值跑）。
func loadConfig(path string) doctorConfig {
	var cfg doctorConfig
	if path == "" {
		return cfg
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "提示：未读到 %s（按默认值运行）：%v\n", path, err)
		return cfg
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "提示：%s 解析失败（按默认值运行）：%v\n", path, err)
		return doctorConfig{}
	}
	return cfg
}

// buildClient 把配置里影响「出站请求长什么样」的字段贴到 client 上。
func buildClient(cfg doctorConfig, timeout time.Duration) *upstream.Client {
	c := upstream.New()
	if timeout > 0 {
		c.HTTP.Timeout = timeout // 探针用整体超时，比 server 的短 RPC 上限更保守
	}
	c.UserAgent = strings.TrimSpace(cfg.Upstream.UserAgent)
	c.ClientVersion = strings.TrimSpace(cfg.Upstream.ClientVersion)
	c.CliVersion = strings.TrimSpace(cfg.Upstream.CliVersion)
	if cfg.Global.Enabled != nil {
		c.GlobalEnabled = *cfg.Global.Enabled
	}
	// 域覆盖（自建反代 / 灰度域名场景）：与 server 同口径，避免探针打默认域名得出假结论。
	if v := strings.TrimSpace(cfg.CN.ChatBase); v != "" {
		c.ChatBaseCN = v
	}
	if v := strings.TrimSpace(cfg.Global.ChatBase); v != "" {
		c.ChatBaseGlobal = v
	}
	if v := strings.TrimSpace(cfg.Global.BillingBase); v != "" {
		c.BillingBaseGlobal = v
	}
	return c
}

// writeReport 落盘健康报告（复用 upstream.SaveHealthReport 的原子写，避免与面板各写一套）。
func writeReport(path string, rep *upstream.HealthReport) {
	if err := upstream.SaveHealthReport(path, rep); err != nil {
		fmt.Fprintf(os.Stderr, "警告：写入健康报告失败：%v\n", err)
	}
}

// printTable 打印人类可读报告。
//
// 采用**竖排**（一账号一节、一探针一行）而不是宽矩阵：探针数会随 P1 档案增长，
// 矩阵在窄终端（80 列以下）必然折行错位；竖排无论加多少探针都不变形，且红黄项
// 的 detail / 漂移项有地方写。横向扫读的便利由每行末尾的状态词承担。
func printTable(rep *upstream.HealthReport) {
	fmt.Printf("上游健康 %s · %d 个账号 · 绿 %d / 黄 %d / 红 %d · 耗时 %dms\n",
		rep.GeneratedAt.Format("2006-01-02 15:04:05"), rep.Summary.Accounts,
		rep.Summary.Green, rep.Summary.Yellow, rep.Summary.Red, rep.DurationMs)

	if len(rep.Accounts) == 0 {
		fmt.Println("（没有账号）")
		return
	}

	for _, acc := range rep.Accounts {
		tags := acc.Realm
		if acc.Enterprise {
			tags += " · 企业版"
		}
		fmt.Printf("\n%s [%s] 总评 %s\n", logfmt.UID8(acc.UID), tags, statusBadge(acc.Status))
		for _, p := range acc.Probes {
			line := fmt.Sprintf("  %s %s (%dms)", statusBadge(p.Status), p.Name, p.LatencyMs)
			if p.Costy {
				line += " [花积分]"
			}
			if p.Detail != "" {
				line += "  " + p.Detail
			}
			fmt.Println(line)
			if len(p.Drift) > 0 {
				fmt.Printf("      漂移项: %s\n", strings.Join(p.Drift, ", "))
			}
		}
	}
}

// statusBadge 把状态渲染成一致宽度的标记，方便肉眼扫。
func statusBadge(status string) string {
	switch status {
	case upstream.ProbeGreen:
		return "green "
	case upstream.ProbeYellow:
		return "yellow"
	case upstream.ProbeRed:
		return "red   "
	default:
		return status
	}
}
