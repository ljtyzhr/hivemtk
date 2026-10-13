#!/usr/bin/env python3
"""量 G4（`check-hardcode-tunable.py`）的**声明面盲区**：包级阈值命名常量里有多少是本门看不见的。

为什么单独一个脚本而不并进 G4：并进判据面就得给每个小写名一个说法（基线或入库），
那是 259 个名字的存量战役，不是一道门的口径修订。本脚本只**出声**，不计红。
它存在的全部理由是把「本门看不见小写名」从一句注释变成一个每次能现跑的读数——
写在门文件头里的静态数字会随代码漂移，而没人会回来订正它。

判据面（与 G4 同源，不另抄一份）：
  - 名字：G4 的 `TUNABLE_SUFFIX`（词缀表原样复用）
  - 值：必须是**数字打头**（含 `int64(20)`、`1 << 20`、`30 * 24 * time.Hour` 这类折叠形式），
    且排除 `iota`（iota 块是枚举不是阈值）
  - 可见性：**用 G4 自己的 `CONST_DECL`** 去匹配复原出来的那一行，匹配上=本门能看见
  - 只有**包级**声明算数：靠大括号/圆括号深度判定顶层，注释与字符串里的括号不参与计数

读数怎么读：
  - `盲区（名字小写）`＝G4 文件头「改个名就能绕开」那条代价的具体数量，属**已知代价**，不是缺陷；
  - `盲区（名字大写却看不见）`＝**值形状没被 CONST_DECL 覆盖**，那是真缺口，逐条列出、逐个补形状或改名；
  - `G4 声明面可见` 与 G4 自己打印的 `Go 常量声明` 不是同一集合：后者含函数内的裸赋值，
    本脚本只数包级，所以两个数不该互相对账。
退出码：0 = 扫到了东西；2 = 扫描面为空或没有 Go 文件（不允许"没扫到"印成"没问题"）。
"""

from __future__ import annotations

import importlib.util
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
for _i, _a in enumerate(sys.argv):
    if _a == "--repo" and _i + 1 < len(sys.argv):
        REPO_ROOT = Path(sys.argv[_i + 1]).resolve()

GATE_REL = Path("scripts") / "check-hardcode-tunable.py"


def load_gate():
    """复用 G4 的判据面，避免两处口径各写一遍就必然漂移。"""
    spec = importlib.util.spec_from_file_location("g4", REPO_ROOT / GATE_REL)
    if spec is None or spec.loader is None:
        print(f"❌ 取不到 G4 脚本：{GATE_REL}")
        raise SystemExit(2)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


# 顶层声明行：`Name = 值`，可带 var/const 前缀、可带类型（`Name int64 = 值`）
DECL = re.compile(r"^(?:const\s+|var\s+)?([A-Za-z_]\w*)\s*(?:[\w\[\]*.<>]+\s+)?=\s*(.+)$")
NUMVAL = re.compile(r"^-?\d[\w.)*(]*(\s*(\*|<<|/)\s*[\w.()]+)*$|^(?:int|int64|int32|float64|float32|uint|uint64|uint32)\s*\(\s*-?\d", re.S)


def blank_out(line: str) -> str:
    """把注释与字符串内容抹成空格（保列宽），让深度计数只看真括号。"""
    out = []
    i, n = 0, len(line)
    while i < n:
        ch = line[i]
        nxt = line[i + 1] if i + 1 < n else ""
        if ch == "/" and nxt == "/":
            out.append(" " * (n - i))
            break
        if ch == "/" and nxt == "*":
            out.append("  ")
            i += 2
            while i < n and not (line[i] == "*" and i + 1 < n and line[i + 1] == "/"):
                out.append(" ")
                i += 1
            out.append("  ")
            i += 2
            continue
        if ch in "\"`'":
            quote = ch
            out.append(" ")
            i += 1
            while i < n:
                if line[i] == "\\":
                    out.append("  ")
                    i += 2
                    continue
                if line[i] == quote:
                    out.append(" ")
                    i += 1
                    break
                out.append(" " if line[i] != "\n" else "\n")
                i += 1
            continue
        out.append(ch)
        i += 1
    return "".join(out)


def main() -> int:
    g4 = load_gate()
    files = []
    for root in g4.GO_ROOTS:
        base = REPO_ROOT / root
        if not base.is_dir():
            continue
        # 排除口径直接调 G4 自己的 is_skipped：两边各写一份就必然漂移。
        files.extend(p for p in sorted(base.rglob("*.go"))
                     if not p.name.endswith("_test.go") and not g4.is_skipped(p))
    if not files:
        print(f"❌ 扫描面为空（根：{[str(r) for r in g4.GO_ROOTS]}）——不是「没有盲区」，是没扫到")
        return 2

    total = visible = blind_lower = blind_upper = 0
    blind_upper_rows = []
    for p in files:
        depth = 0
        in_block = False
        for raw in p.read_text(encoding="utf-8", errors="replace").splitlines():
            line = blank_out(raw).rstrip()
            stripped = line.strip()
            if stripped:
                opener = bool(re.match(r"^(?:const|var)\s*\($", stripped))
                # 包级声明的两种位置：顶层单行（depth 0），以及顶层 const/var 块内（depth 1）。
                # 块内那一批是多数量的来源（`missingThreshold = int64(20)` 就住在 var 块里），
                # 只数顶层单行会把整块漏掉。
                if (depth == 0 and not opener) or (in_block and depth == 1):
                    m = DECL.match(stripped)
                    if m and NUMVAL.match(m.group(2).strip()) and g4.TUNABLE_SUFFIX.search(m.group(1)):
                        total += 1
                        # 可见性用**原始那一行**去问 G4 的 CONST_DECL，不用去掉类型的复原行：
                        # 复原行会把「`Name Type = 值` 这种带类型的声明本门根本看不见」这件事
                        # 掩盖掉（CONST_DECL 要求名字后面直接跟 `=`）。
                        probe = f"    {stripped}"
                        if g4.CONST_DECL.match(probe):
                            visible += 1
                        elif m.group(1)[0].isupper():
                            blind_upper += 1
                            blind_upper_rows.append((p.relative_to(REPO_ROOT), m.group(1), stripped))
                        else:
                            blind_lower += 1
                if depth == 0 and opener:
                    in_block = True
                depth += line.count("(") + line.count("{") - line.count(")") - line.count("}")
                if depth <= 0:
                    depth = 0
                    in_block = False
    print(f"扫描面: {len(files)} 个 Go 文件（口径与 G4 同源：{[str(r) for r in g4.GO_ROOTS]}，排测试与 SKIP_SEGMENTS）")
    print(f"读数: 包级阈值命名数值常量 {total}  其中 G4 声明面可见 {visible}  "
          f"盲区（名字小写）{blind_lower}  盲区（名字大写却看不见＝值形状未覆盖）{blind_upper}")
    for f, name, val in blind_upper_rows:
        print(f"  BLIND_UPPER: {f}: {name} = {val[:60]}")
    print("说明: 小写名那一批是 G4 文件头「改个名就能绕开」的已知代价，本脚本只量化不处置；"
          "大写却看不见的那些是真缺口，要么补 CONST_DECL 形状，要么解释为什么不算阈值。")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
