#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把 router-list.md 里指定路由的「引导」列标成 G（已写页面导览）。

为什么要单独一个脚本：这一列是人工点验的结论（浏览器里点过、文案对得上），
而清单是 `scripts/gen_router_list.py` 生成出来的。直接手改 md 可以，但下一次
重跑生成脚本时，「保留既有状态」那套逻辑只认它自己写出的表格格式——手改容易
改错列（多一格少一格都会让状态串位）。

用法：
    python3 scripts/mark_guide_done.py /geo-tools/brand-config /geo-tools/config ...
    python3 scripts/mark_guide_done.py --from-file routes.txt
"""
import argparse
import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "router-list.md")


def mark(paths):
    if not os.path.exists(OUT):
        raise SystemExit(f"找不到 {OUT}")
    with open(OUT, encoding="utf-8") as fh:
        lines = fh.read().split("\n")

    want = set(paths)
    hit = set()
    for i, line in enumerate(lines):
        if not line.startswith("|"):
            continue
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        if len(cells) < 9 or not cells[0].isdigit():
            continue
        path = cells[2].strip("`")
        if path in want and cells[8] != "G":
            cells[8] = "G"
            lines[i] = "| " + " | ".join(cells) + " |"
            hit.add(path)

    with open(OUT, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))

    missing = want - hit
    print(f"标 G：{len(hit)} 条")
    if missing:
        print(f"⚠️ 在 router-list.md 里没找到这些路径（可能路径写错）：{sorted(missing)}")
        return 1
    return 0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("paths", nargs="*")
    ap.add_argument("--from-file")
    args = ap.parse_args()
    paths = list(args.paths)
    if args.from_file:
        with open(args.from_file, encoding="utf-8") as fh:
            paths += [l.strip() for l in fh if l.strip() and not l.startswith("#")]
    if not paths:
        raise SystemExit(__doc__)
    sys.exit(mark(paths))


if __name__ == "__main__":
    main()
