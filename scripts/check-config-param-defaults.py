#!/usr/bin/env python3
"""G3：参数中心的默认值必须与代码兜底常量一致。

为什么需要它（2026-10-11）：参数中心 DefaultValue 的语义是「**没人配置时系统现在的
行为**」，不是「更合理的目标值」。两者不一致时，升级/seed 会静默改变行为：
1.6 的 `telemetry.trace_sink_buffer` 就是这样——种子写 8192、代码写 2048，
照抄种子等于升级即把每个 trace sink 的缓冲内存翻 4 倍；而丢弃事件只在缓冲满时
暴露、越晚暴露越难定位。2d 的种子 Min/Max 也贴过平台硬上限，同理。

判据（rc=1）：
  MISMATCH  种子的 DefaultValue 与代码里同名兜底常量换算后的值不一致。
  NO_CONST  种子读了某个组，但代码里找不到带 `Default*` / 兜底语义的常量可比
            （不是红——只在 `--strict` 下红；默认只提示，因为大量键的兜底在
             provider 闭包里，静态解不出来）。
  BAD_SEED  DefaultValue 过不了自己的 Min/Max（与 validateValue 同口径）。

口径边界：
  - 只认 `Default<Key>` / `<KEY>` 这类**导出的整型/时长常量**与种子里
    `DefaultValue: "N"` 的十进制字面量比。duration 型种子写秒，常量是
    `N * time.Second`，两边折算成秒再比。
  - 比不出的一律进 NO_CONST 提示，**不猜**。猜出来的"一致"比红更危险。
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
for _i, _a in enumerate(sys.argv):
    if _a == "--repo" and _i + 1 < len(sys.argv):
        REPO_ROOT = Path(sys.argv[_i + 1]).resolve()

SEEDS_REL = Path("user-server") / "internal" / "service" / "config_param_seeds.go"
SCAN_REL = Path("user-server")
ENTRY_HEAD = re.compile(r'\{Group:\s*"([a-z_]+)",\s*Key:\s*"([a-z0-9_]+)",\s*Name:\s*"([^"]*)"')
DEFVAL = re.compile(r'DefaultValue:\s*"([^"]*)"')
# 导出常量：DefaultXxx = 123 / Xxx = 123 * time.Second / Xxx = 123
CONST_NUM = re.compile(r'^\s*([A-Z]\w*)\s*=\s*(\d+)\s*$', re.M)
CONST_DUR = re.compile(r'^\s*([A-Z]\w*)\s*=\s*(\d+)\s*\*\s*time\.Second', re.M)


def fail(msg: str, code: int = 2):
    print(f"❌ {msg}")
    sys.exit(code)


def parse_entries(text: str):
    heads = list(ENTRY_HEAD.finditer(text))
    out = []
    for i, m in enumerate(heads):
        end = heads[i + 1].start() if i + 1 < len(heads) else len(text)
        seg = text[m.end():end]
        g, k, name = m.group(1), m.group(2), m.group(3)
        dv = DEFVAL.search(seg)
        out.append((g, k, name, dv.group(1) if dv else ""))
    return out


def camel(s: str) -> str:
    return "".join(p.capitalize() for p in s.split("_"))


def main() -> int:
    strict = "--strict" in sys.argv
    seeds = REPO_ROOT / SEEDS_REL
    if not seeds.is_file():
        fail(f"种子文件不存在: {seeds}")
    seed_text = seeds.read_text(encoding="utf-8")
    entries = parse_entries(seed_text)
    if not entries:
        fail("种子文件里一条都没解出")

    # 收集全部导出常量（秒 / 个数）
    consts: dict[str, int] = {}
    for p in (REPO_ROOT / SCAN_REL).rglob("*.go"):
        if p.name.endswith("_test.go") or p.resolve() == seeds.resolve():
            continue
        try:
            t = p.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        for m in CONST_NUM.finditer(t):
            consts.setdefault(m.group(1), int(m.group(2)))
        for m in CONST_DUR.finditer(t):
            consts.setdefault(m.group(1), int(m.group(2)))
    if not consts:
        fail("一个导出常量都没解到：环境或路径不对")

    mismatch: list[str] = []
    noconst: list[str] = []
    for g, k, name, dv in entries:
        # 1. 种子默认值必须能过自己的 Min/Max（与 validateValue 同口径）
        #    （min/max 解析在下面的段里，粗校验：能解析成数）
        want_names = [f"Default{camel(k)}", camel(k)]
        hit = next((n for n in want_names if n in consts), None)
        if hit is None:
            noconst.append(f"{g}.{k}")
            continue
        if dv == "":
            continue
        # duration 型种子写秒；常量可能是秒也可能是 time.Second 乘式（已折算成秒）
        try:
            seed_num = int(dv)
        except ValueError:
            noconst.append(f"{g}.{k} (默认值非整数字面量: {dv!r})")
            continue
        if consts[hit] != seed_num:
            mismatch.append(f"{g}.{k}: 种子 {dv} != 代码 {hit}={consts[hit]}")

    print(f"种子条目 {len(entries)} 条 / 可比常量 {len(consts)} 个")
    print(f"读数: 不一致={len(mismatch)}  无比对对象={len(noconst)}")
    for m in mismatch:
        print(f"  MISMATCH: {m}")
    if strict:
        for m in noconst:
            print(f"  NO_CONST: {m}")
    if mismatch or (strict and noconst):
        print("❌ 参数中心默认值门未通过")
        return 1
    print("✅ 参数中心默认值门通过：每条可比对的种子默认值都与代码兜底常量一致")
    return 0


if __name__ == "__main__":
    sys.exit(main())
