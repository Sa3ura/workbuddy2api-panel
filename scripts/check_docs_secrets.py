#!/usr/bin/env python3
"""扫描 docs/ 与根目录 *.md，发现疑似凭证/个人信息即失败。

用途：维护文档会记录大量上游行为，抄录请求示例时极易把真实的 token、手机号、
昵称一起带进来。本脚本是入库前的最后一道闸，CI 与本地提交前都跑。

用法：
    python scripts/check_docs_secrets.py            # 扫默认范围
    python scripts/check_docs_secrets.py --quiet    # 只输出结论
    python scripts/check_docs_secrets.py path ...   # 扫指定文件/目录

退出码：0 = 干净；1 = 命中可疑内容；2 = 参数/环境错误

判据设计：
    - 命中"明显的凭证形态"（Bearer + 长串、sk- 长串、32+ 位十六进制/Base64）即失败；
    - 占位符不算（REDACTED / xxx / <...> / 示例 / test / your-key 等）；
    - 手机号按中国 11 位规则匹配，但排除文档里常见的假号（13800138000 等）。
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent

# 允许放过的高频占位符（大小写不敏感）
PLACEHOLDER = re.compile(
    r"^(?:redacted|xxx+|\*+|\.{3}|<[^>]*>|\$\{[^}]*\}|your[-_]?\w*|example\w*|test[-_]?\w*|dummy\w*|placeholder\w*|sk-xxx+)$",
    re.IGNORECASE,
)

RULES = [
    # (名称, 正则, 取值组)
    ("Bearer 令牌", re.compile(r"Bearer\s+([A-Za-z0-9\-._~+/]{20,}=*)"), 1),
    ("sk- 密钥", re.compile(r"\b(sk-[A-Za-z0-9]{16,})"), 1),
    ("疑似 accessToken 赋值", re.compile(r'"?access_?token"?\s*[:=]\s*"([^"]{12,})"', re.IGNORECASE), 1),
    ("疑似 refreshToken 赋值", re.compile(r'"?refresh_?token"?\s*[:=]\s*"([^"]{12,})"', re.IGNORECASE), 1),
    ("32+ 位十六进制串", re.compile(r"\b([0-9a-fA-F]{32,})\b"), 1),
    ("手机号", re.compile(r"\b(1[3-9]\d{9})\b"), 1),
    ("Cookie 段", re.compile(r"(?:^|;|\s)(?:uin|skey|p_skey|RK)=([A-Za-z0-9%\-_.]{8,})", re.IGNORECASE), 1),
]

# 文档里可以合法出现的假手机号 / 测试号
PHONE_ALLOW = {
    "13800138000",
    "13800138001",
    "13900000000",
    "10000000000",
}


def default_targets() -> list[Path]:
    targets: list[Path] = []
    docs = REPO / "docs"
    if docs.is_dir():
        targets.extend(sorted(docs.rglob("*.md")))
        targets.extend(sorted(docs.rglob("*.json")))
    for name in ("README.md", "MAINTAINING.md", "EXEC-PLAN.md", "CHANGELOG.md", "CONTRIBUTING.md"):
        p = REPO / name
        if p.is_file():
            targets.append(p)
    return targets


def scan_file(path: Path) -> list[str]:
    hits: list[str] = []
    try:
        text = path.read_text(encoding="utf-8", errors="replace")
    except OSError as exc:  # 权限/编码问题不静默
        return [f"{path}: 读取失败（{exc}）"]

    for lineno, line in enumerate(text.splitlines(), 1):
        for label, pattern, group in RULES:
            for m in pattern.finditer(line):
                value = m.group(group)
                if PLACEHOLDER.match(value.strip()):
                    continue
                if label == "手机号" and value in PHONE_ALLOW:
                    continue
                rel = path.relative_to(REPO) if path.is_relative_to(REPO) else path
                hits.append(f"{rel}:{lineno}: [{label}] {value[:12]}…")
    return hits


def main() -> int:
    ap = argparse.ArgumentParser(description="扫描维护文档中的疑似凭证与个人信息")
    ap.add_argument("paths", nargs="*", help="要扫描的文件或目录（默认 docs/ 与根目录维护文档）")
    ap.add_argument("--quiet", action="store_true", help="只在失败时输出")
    args = ap.parse_args()

    targets: list[Path] = []
    if args.paths:
        for raw in args.paths:
            p = Path(raw)
            if not p.is_absolute():
                p = (Path.cwd() / p).resolve()
            if p.is_dir():
                targets.extend(sorted(p.rglob("*.md")))
                targets.extend(sorted(p.rglob("*.json")))
            elif p.is_file():
                targets.append(p)
            else:
                print(f"路径不存在：{raw}", file=sys.stderr)
                return 2
    else:
        targets = default_targets()

    if not targets:
        print("没有找到要扫描的文档（docs/ 为空？）", file=sys.stderr)
        return 0

    all_hits: list[str] = []
    for t in targets:
        all_hits.extend(scan_file(t))

    if all_hits:
        print("发现疑似凭证 / 个人信息，禁止入库：", file=sys.stderr)
        for h in all_hits:
            print("  " + h, file=sys.stderr)
        print(
            "\n处理方式：替换为 REDACTED / <uid> / <token> 等占位符后重跑。\n"
            "确属无风险的假值（如文档示例手机号），请加进脚本的 PHONE_ALLOW。",
            file=sys.stderr,
        )
        return 1

    if not args.quiet:
        print(f"doc-secrets: clean（扫描 {len(targets)} 个文件）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
