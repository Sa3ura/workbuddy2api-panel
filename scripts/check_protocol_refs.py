#!/usr/bin/env python3
"""检查代码注释里指向外部材料的引用，是否都带了本地锚点。

背景（docs/protocol/README.md §0）：本项目原始逆向文档从未进入版本库，代码里
`sse.ts:91-123`、`data/desktop-task-protocol.md §7.3`、`.claude/reports/…` 这类引用
指向的是**永久不可得**的材料。这些引用保留是历史证据，但必须同时给出本地落点，
否则未来的维护者会以为仓库里有一份文件被误删，然后白找半天。

规则：出现外部文档引用（*.md / *.ts）的那一行，或其下 2 行内，必须出现
`docs/protocol/`（指向档案或待重建清单）。

用法：
    python scripts/check_protocol_refs.py            # 检查
    python scripts/check_protocol_refs.py --fix-hint # 顺带打印建议锚点

退出码：0 = 全部有锚点；1 = 存在裸引用；2 = 参数/环境错误
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent

# 外部文档引用：文件名后必须跟「非标识符字符」，避免把 ctx.mdMatch 这类 JS 变量算进来
REF_RE = re.compile(r"(?<![\w./-])([A-Za-z][\w\-]*\.(?:md|ts))(?=[:：\s,，。）)\]]|$)")

# 允许的例外：这些文件在仓库里真实存在（不指向不可得材料）
ALLOW = {
    "defaultprompt.md",  # internal/prompt/defaultprompt.md 已入库
    "README.md",
    "MAINTAINING.md",
    "EXEC-PLAN.md",
    "CHANGELOG.md",
    "CONTRIBUTING.md",
}

ANCHOR = "docs/protocol/"
LOOKAHEAD = 10  # 锚点允许出现在引用行下方若干行内（文件头注释块可能较长）


def iter_go_files() -> list[Path]:
    out: list[Path] = []
    for root in ("cmd", "internal"):
        d = REPO / root
        if d.is_dir():
            out.extend(sorted(p for p in d.rglob("*.go") if p.is_file()))
    return out


def check(path: Path) -> list[str]:
    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError as exc:
        return [f"{path}: 读取失败（{exc}）"]

    problems: list[str] = []
    for i, line in enumerate(lines):
        refs = [m.group(1) for m in REF_RE.finditer(line)]
        if not refs:
            continue
        if all(r in ALLOW for r in refs):
            continue
        if ANCHOR in line:
            continue
        window = "\n".join(lines[i : i + 1 + LOOKAHEAD])
        if ANCHOR in window:
            continue
        # 只关心第一处未带锚点的引用（同一行重复引用不重复报）
        rel = path.relative_to(REPO) if path.is_relative_to(REPO) else path
        problems.append(f"{rel}:{i + 1}: 裸引用 {refs[0]}（缺 docs/protocol/ 锚点）")
    return problems


def main() -> int:
    ap = argparse.ArgumentParser(description="检查外部文档引用是否都带本地锚点")
    ap.add_argument("--fix-hint", action="store_true", help="打印建议的锚点写法")
    args = ap.parse_args()

    files = iter_go_files()
    if not files:
        print("没有找到 Go 源文件", file=sys.stderr)
        return 2

    problems: list[str] = []
    for f in files:
        problems.extend(check(f))

    if problems:
        print("存在未带本地锚点的外部文档引用：", file=sys.stderr)
        for p in problems:
            print("  " + p, file=sys.stderr)
        if args.fix_hint:
            print(
                "\n建议在引用行下方补一行：\n"
                "  // 原始材料不可得（从未入版本库）；结论见 docs/protocol/<档案>.md#<锚点>\n"
                "  尚未重建的档案请指向 docs/protocol/README.md §2 待重建清单。",
                file=sys.stderr,
            )
        return 1

    print(f"protocol-refs: clean（扫描 {len(files)} 个 Go 文件，全部引用均有本地锚点）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
