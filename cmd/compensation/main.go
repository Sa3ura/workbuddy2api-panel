// Command compensation 限时补偿领取（先查后领）。
//
// 背景：官方补偿（例：「模型服务异常额外补偿 500 积分」）在账号侧可见且有领取截止日，
// 过期即作废。网关内部只在签到排程末尾顺带领一次（scheduler.claimCompensationQuiet），
// 面板有「官方补偿」卡片与 compensation_all 入口，但没有命令行通道——本工具补这一块，
// 用于：面板没起、只想快速领一次、或在服务器上 cron 化。
//
// 语义与面板/调度器完全一致（同为「先查 compensation-status 再决定是否领」）：
//   - 无活动 / 已领 / credit_num<=0 → 跳过，不算错误
//   - 查询失败与领取失败**分别显式报错**（旧实现把三态压成一种，带截止日的补偿会静默过期）
//
// 写操作默认 **dry-run**（只查不领），必须显式 -yes 才真的领——与 scripts/probe_active.py 同约定。
//
// 用法：
//
//	go run ./cmd/compensation                  # dry-run：列出待领补偿与截止时间
//	go run ./cmd/compensation -yes             # 真的领
//	go run ./cmd/compensation -uid <uid> -yes  # 只领一个号
//
// 退出码：0 成功（含"无待领"）；1 有账号查询或领取失败；2 参数/凭证问题。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/logfmt"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

func main() {
	var (
		authDir = flag.String("auth-dir", "./auths", "凭证目录")
		uid     = flag.String("uid", "", "只处理指定 uid")
		doit    = flag.Bool("yes", false, "真的领取（默认 dry-run，只查不领）")
		timeout = flag.Duration("timeout", 60*time.Second, "整体超时")
	)
	flag.Parse()

	accounts, err := auth.LoadDir(*authDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取凭证目录失败 %s: %v\n", *authDir, err)
		os.Exit(2)
	}
	if len(accounts) == 0 {
		fmt.Fprintf(os.Stderr, "凭证目录 %s 里没有账号（workbuddy*.json）\n", *authDir)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := upstream.New()
	client.HTTP.Timeout = *timeout

	mode := "dry-run（只查不领，加 -yes 才真领）"
	if *doit {
		mode = "执行领取"
	}
	fmt.Printf("限时补偿 · %d 个账号 · %s\n\n", len(accounts), mode)

	var failed, claimed, pending int
	for _, a := range accounts {
		if a == nil {
			continue
		}
		if *uid != "" && a.UID != *uid {
			continue
		}
		if err := ctx.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "超时中止：%v\n", err)
			failed++
			break
		}

		label := logfmt.Label(a.UID, a.Nickname)
		if a.IsEnterprise() {
			// 企业号无补偿体系（上游 400 code 10001），跳过而不是记失败——避免每天刷错误行。
			fmt.Printf("%s  跳过（企业版无补偿体系）\n", label)
			continue
		}

		st, err := client.GetCompensationStatus(a)
		if err != nil {
			fmt.Printf("%s  ❌ 状态查询失败：%v\n", label, err)
			failed++
			continue
		}
		if !st.Pending() {
			// 三态合一但**只在这里**合并：无活动 / 已领 / 无额度都确实无需动作。
			switch {
			case !st.Active:
				fmt.Printf("%s  无生效活动\n", label)
			case st.Claimed:
				fmt.Printf("%s  本活动已领取（额度 %d，截止 %s）\n", label, st.CreditNum, st.EndTime)
			default:
				fmt.Printf("%s  活动存在但无额度（credit_num=%d）\n", label, st.CreditNum)
			}
			continue
		}

		pending++
		deadline := st.EndTime
		warn := ""
		if left, ok := timeLeft(st.EndTime); ok {
			if left <= 48*time.Hour {
				warn = fmt.Sprintf("  ⚠ 剩 %s", humanDuration(left))
			}
		}
		fmt.Printf("%s  待领 %d 积分 · 截止 %s%s\n", label, st.CreditNum, deadline, warn)

		if !*doit {
			continue
		}

		got, err := client.ClaimCompensation(a)
		if err != nil {
			fmt.Printf("%s  ❌ 领取失败：%v\n", label, err)
			failed++
			continue
		}
		claimed++
		fmt.Printf("%s  ✅ 已领取 %d 积分\n", label, got)
	}

	fmt.Println()
	switch {
	case failed > 0:
		fmt.Printf("汇总：待领 %d · 已领 %d · 失败 %d\n", pending, claimed, failed)
		os.Exit(1)
	case !*doit && pending > 0:
		fmt.Printf("汇总：待领 %d（dry-run 未领取；确认后加 -yes）\n", pending)
	default:
		fmt.Printf("汇总：待领 %d · 已领 %d\n", pending, claimed)
	}
}

// timeLeft 解析上游 "2006-01-02 15:04:05" 无时区字符串，按 UTC+8 解释（与上游一致），
// 返回距截止的剩余时长。解析失败时 ok=false（不臆造截止紧迫性）。
func timeLeft(endTime string) (time.Duration, bool) {
	if endTime == "" {
		return 0, false
	}
	cst := time.FixedZone("CST", 8*3600)
	t, err := time.ParseInLocation("2006-01-02 15:04:05", endTime, cst)
	if err != nil {
		return 0, false
	}
	return time.Until(t), true
}

// humanDuration 把剩余时长压成 "23h59m" / "45m" 形式。
func humanDuration(d time.Duration) string {
	if d < 0 {
		return "已过期"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}
