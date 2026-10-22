#!/usr/bin/env python3
"""应用降档预设到 config.json（只合并预设里出现的键，其余键原样保留）。

用法：
    python3 scripts/apply_preset.py presets/throttle.json            # 预览（不写盘）
    python3 scripts/apply_preset.py presets/throttle.json --apply    # 真的写
    python3 scripts/apply_preset.py --list                           # 列出可用预设

设计取舍：
  * **默认只预览**。降档是在紧张时刻做的操作，最怕手滑写坏 config.json —— 所以
    不加 --apply 就只打印将要改动的键，什么都不写。
  * **写前必留备份**。config.json.bak-<时间戳>，删不掉也不碍事（几十 KB）。
  * **只合并预设里出现的键**（深合并，预设赢）。预设是"差异键清单"，不是整份配置——
    这样用户的 api_key、auth_dir、模型隐藏等个性化设置永远不会被预设覆盖掉。
  * `_` 开头的键是注释（_when/_tradeoff/_comment...），跳过不写。

为什么不用面板的配置页做这件事：面板保存是"整份覆盖"，从面板切档要先手工改十几个
字段；而脚本一次搞定且可审计（diff 打印旧值 → 新值）。
"""

import json
import shutil
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CONFIG = ROOT / "config.json"
PRESET_DIR = ROOT / "presets"


def load_json(path: Path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def merge_diff(target: dict, overlay: dict, prefix: str = "") -> list[tuple[str, object, object]]:
    """把 overlay 深度合并进 target，返回改动清单 [(键路径, 旧值, 新值)]。

    只处理 overlay 中出现的键；overlay 里以 `_` 开头的键视为注释跳过。
    """
    changes = []
    for k, v in overlay.items():
        if k.startswith("_"):
            continue
        path = f"{prefix}{k}"
        if isinstance(v, dict) and isinstance(target.get(k), dict):
            changes.extend(merge_diff(target[k], v, path + "."))
            continue
        old = target.get(k)
        if old != v:
            changes.append((path, old, v))
        target[k] = v
    return changes


def main(argv: list[str]) -> int:
    if "--list" in argv or len(argv) < 2:
        names = sorted(p.stem for p in PRESET_DIR.glob("*.json"))
        print("可用预设：" + "、".join(names))
        print(f"\n用法：python3 scripts/apply_preset.py presets/<档位>.json [--apply]")
        return 0 if "--list" in argv else 2

    preset_path = Path(argv[1])
    if not preset_path.is_absolute():
        preset_path = ROOT / preset_path
    if not preset_path.exists() and (PRESET_DIR / Path(argv[1]).name).exists():
        preset_path = PRESET_DIR / Path(argv[1]).name
    if not preset_path.exists():
        print(f"预设不存在：{argv[1]}", file=sys.stderr)
        return 2

    if not CONFIG.exists():
        print(f"找不到 {CONFIG}（先把 config.example.json 复制成 config.json）", file=sys.stderr)
        return 2

    cfg = load_json(CONFIG)
    preset = load_json(preset_path)

    print(f"预设：{preset_path.name}")
    for meta in ("_when", "_tradeoff", "_recover"):
        if meta in preset:
            print(f"  {meta[1:]}: {preset[meta]}")

    changes = merge_diff(cfg, preset)
    if not changes:
        print("\n没有需要改的键（当前配置已与预设一致）。")
        return 0

    print(f"\n将修改 {len(changes)} 个键：")
    for path, old, new in changes:
        print(f"  {path}: {old!r} -> {new!r}")

    if "--apply" not in argv:
        print("\n（预览模式，未写盘。加 --apply 真正应用）")
        return 0

    backup = CONFIG.with_name(CONFIG.name + ".bak-" + time.strftime("%Y%m%d_%H%M%S"))
    shutil.copy2(CONFIG, backup)
    with open(CONFIG, "w", encoding="utf-8") as f:
        json.dump(cfg, f, ensure_ascii=False, indent=2)
        f.write("\n")
    print(f"\n已写入 {CONFIG.name}（备份：{backup.name}）")
    print("生效方式：pool/schedule 类键热生效；改了 listen/auth_dir 之类才需要重启。")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
