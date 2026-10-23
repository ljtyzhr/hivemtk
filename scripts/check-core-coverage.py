#!/usr/bin/env python3
"""A4-a：核心域分档覆盖率门 —— 每个核心包有自己的底线，而不是全仓一个 22%。

为什么需要它（以及为什么不是"再加一道总覆盖率门"）：
  CI 里已经有 `Coverage threshold (OPT-CI-06)`（FLOOR=20 阻断 + TARGET=60 提示）与
  `codecov.yml` 的 project target。这两者的口径都是**全仓一个数**，而全仓平均值的
  结构性问题正是它看不见的：
    · 一个核心包（如 `internal/service` 的触达链路）从 45% 掉到 25%，只要边缘包涨回来，
      总数不变 ⇒ 门绿灯，而真正会赔钱的地方已经没人测了；
    · 反过来，为了把总数从 22% 推到 30%，最省力的做法是给一堆无人调用的边缘包补测试，
      核心包一个用例都不用加。
  所以本门把"覆盖率"从**一个数**拆成**一组数**：核心域每个包一条自己的底线，
  只许涨不许跌；非核心包不管（它们由总覆盖率门与 codecov 趋势负责）。

判据（任一不成立即 rc=1）：
  A. 核心包在 profile 里且实际覆盖率 < 基线底线 ⇒ `BELOW_FLOOR` 红；
  B. 核心包在 profile 里但基线没登记 ⇒ `UNREGISTERED` 红（新增核心包必须显式定底线，
     否则它就是"刚出生就没人管"的那个包）；
  C. 基线登记了核心包但 profile 里没有 ⇒ `NOT_IN_PROFILE` 红
     （**没测到 ≠ 覆盖率 0**：这通常是包被排除出测试集合、或被 skip 掉了）；
  D. 基线登记的包在仓库里已不存在 ⇒ `STALE` 红（防基线烂成永久豁免）；
  E. profile 缺失/为空/解析不出任何语句 ⇒ rc=2（不冒充绿）。

基线（`scripts/core-coverage.baseline`）是**棘轮**：`--refresh` 只会上调，
算出比现有更低的值时保持原值并打印"拒绝下调"。要真的下调，得写 `--allow-drop <包名>`
并在提交信息里说明原因——下调覆盖率是一次需要签字的决定，不该是刷基线的副作用。

口径边界：
  - 计的是**语句覆盖率**（`go test -covermode=atomic` 的 `stmts` 列），与 CI 现有的
    `go tool cover -func` 同源，但按**包**聚合而不是按文件/全局。
  - 只统计 `CORE_PREFIXES` 命中的包。哪些算核心域是**显式清单**，不是猜的：
    触达/审批/客户/知识库这些会直接导致"发错消息、越权、赔钱"的链路。
    清单变了（新核心包出现），本门的 UNREGISTERED 会替你把新包顶出来。
  - 一个包被多个前缀命中只算一次；前缀按**最长前缀**匹配语义在代码里是"任一命中"，
    因为 CORE_PREFIXES 之间刻意不重叠。
  - 覆盖率数字依赖当轮 profile 的完整性。**本门不校验 profile 完整性**
    （那是 CI "Coverage (Go)" 步骤里 `reported.txt` 对账的活）；profile 缺包时
    本门给的是 `NOT_IN_PROFILE`，不会静默当成 0%。

执行入口：
  本地 `python3 scripts/check-core-coverage.py --profile user-server/coverage/go-cover.out`
  CI：`user-server-ci.yml` 的 coverage 作业，在 "Coverage threshold (OPT-CI-06)" 之后。
牙齿：`python3 scripts/check-core-coverage.py --self-check`（8 枚控制）。
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
for _i, _a in enumerate(sys.argv):
    if _a == "--repo" and _i + 1 < len(sys.argv):
        REPO_ROOT = Path(sys.argv[_i + 1]).resolve()

DEFAULT_PROFILE = Path("user-server") / "coverage" / "go-cover.out"
BASELINE_REL = Path("scripts") / "core-coverage.baseline"

# 核心域：命中任一条即受本门管辖。刻意用**完整包路径前缀**而不是业务词
# （`reach` 这种词在别的仓里到处都是），这样清单本身可审计。
CORE_PREFIXES = [
    "hivemtk-user/internal/service",
    "hivemtk-user/internal/repository",
    "hivemtk-user/internal/controller",
    "hivemtk-user/internal/model",
    "hivemtk-user/internal/reach",
    "hivemtk-user/internal/approval",
    "hivemtk-user/internal/browser_automation/",
    "hivemtk-user/internal/aiagent/",
]

PROFILE_LINE = re.compile(r"^(.+?):(\d+)\.(\d+),(\d+)\.(\d+)\s+(\d+)\s+(\d+)$")


class Finding:
    def __init__(self, code: str, msg: str):
        self.code = code
        self.msg = msg

    def __str__(self) -> str:
        return f"  {self.code}: {self.msg}"


def is_core(pkg: str) -> bool:
    return any(pkg == p or pkg.startswith(p.rstrip("/") + "/") for p in CORE_PREFIXES)


def parse_profile(text: str) -> dict[str, tuple[int, int]]:
    """profile → {包路径: (covered_stmts, total_stmts)}。

    只按 `mode:` 之后的块统计；重复出现的语句区间（同包多个测试文件）按语句数累加，
    `count>0` 记为覆盖。这与 `go tool cover -func` 的包级聚合口径一致。
    """
    per_pkg: dict[str, list[tuple[int, int, str]]] = {}
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("mode:"):
            continue
        m = PROFILE_LINE.match(line)
        if not m:
            continue
        file, s1, s2, e1, e2, stmts, count = m.groups()
        pkg = file.rsplit("/", 1)[0] if "/" in file else "."
        block = f"{file}:{s1}.{s2},{e1}.{e2}"
        per_pkg.setdefault(pkg, []).append((int(stmts), int(count), block))
    out: dict[str, tuple[int, int]] = {}
    for pkg, rows in per_pkg.items():
        seen: dict[str, tuple[int, int]] = {}
        for stmts, count, block in rows:
            prev = seen.get(block)
            # 同一语句块在多个 profile 行里重复出现时取 max(count)，不重复计语句数，
            # 否则多包测试同一文件会把分母算大、覆盖率被稀释。
            if prev is None or count > prev[1]:
                seen[block] = (stmts, count)
        total = sum(s for s, _ in seen.values())
        covered = sum(s for s, c in seen.values() if c > 0)
        out[pkg] = (covered, total)
    return out


def pct(covered: int, total: int) -> float:
    return 100.0 * covered / total if total else 0.0


def parse_baseline(text: str) -> tuple[dict[str, int], list[str]]:
    """基线：`package | floor_percent`。返回 (行表, 格式错误行)。"""
    rows: dict[str, int] = {}
    bad: list[str] = []
    for ln in text.splitlines():
        s = ln.strip()
        if not s or s.startswith("#"):
            continue
        parts = [x.strip() for x in s.split("|")]
        if len(parts) != 2:
            bad.append(ln)
            continue
        try:
            rows[parts[0]] = int(round(float(parts[1].rstrip("%"))))
        except ValueError:
            bad.append(ln)
    return rows, bad


def evaluate(
    *, stats: dict[str, tuple[int, int]], baseline: dict[str, int], baseline_bad: list[str],
    core_exists=None,
) -> list[Finding]:
    findings: list[Finding] = []

    for ln in baseline_bad:
        findings.append(Finding("BASELINE_FORMAT", f"基线行不是 `package | 百分比`：{ln}"))

    for pkg in sorted(stats):
        if not is_core(pkg):
            continue
        covered, total = stats[pkg]
        floor = baseline.get(pkg)
        if floor is None:
            findings.append(Finding(
                "UNREGISTERED",
                f"{pkg}: 核心域包未登记底线（实际 {pct(covered, total):.1f}%）—— "
                f"在 {BASELINE_REL} 加一行 `{pkg} | <当前值向下取整>`，"
                f"新增核心包必须有底线，否则它就是唯一没人管的那个"))
            continue
        actual = pct(covered, total)
        if actual + 1e-9 < floor:
            findings.append(Finding(
                "BELOW_FLOOR",
                f"{pkg}: {actual:.1f}% < 底线 {floor}% —— 覆盖率退了（{covered}/{total} 语句）"))

    for pkg, floor in sorted(baseline.items()):
        if pkg in stats:
            continue
        if core_exists is not None and not core_exists(pkg):
            findings.append(Finding(
                "STALE", f"{pkg}: 基线登记了但仓库里已没有这个包（底线 {floor}%）—— 删掉这一行"))
            continue
        findings.append(Finding(
            "NOT_IN_PROFILE",
            f"{pkg}: 基线有底线 {floor}%，但这一轮 profile 里没有它的任何语句 —— "
            f"没测到不等于覆盖率 0：先确认它为什么没跑（排除出测试集合？整包 skip？）"))
    return findings


def core_exists_in_repo(pkg: str) -> bool:
    """包路径 → 源码目录是否还在（`hivemtk-user/internal/x` → `user-server/internal/x`）。"""
    if not pkg.startswith("hivemtk-user/"):
        return False
    return (REPO_ROOT / "user-server" / pkg[len("hivemtk-user/"):]).is_dir()


def read_profile(path: Path) -> dict[str, tuple[int, int]]:
    if not path.is_file():
        fail(f"profile 不存在：{path}（先跑 CI 的 Coverage (Go) 步骤或本地 "
             f"`go test ./... -coverprofile=coverage/go-cover.out -covermode=atomic`）")
    text = path.read_text(encoding="utf-8", errors="replace")
    stats = parse_profile(text)
    if not stats:
        fail(f"profile 里解析不出任何语句：{path}（空 profile / 头注释之外没有数据行 ⇒ 不冒充绿）")
    return stats


def refresh(path: Path) -> int:
    stats = read_profile(path)
    bl_path = REPO_ROOT / BASELINE_REL
    existing: dict[str, int] = {}
    if bl_path.is_file():
        existing, _ = parse_baseline(bl_path.read_text(encoding="utf-8"))
    rows: list[tuple[str, int]] = []
    kept: list[str] = []
    for pkg in sorted(stats):
        if not is_core(pkg):
            continue
        covered, total = stats[pkg]
        new_floor = int(pct(covered, total) // 1)  # 向下取整：底线不许比实测高
        old = existing.get(pkg)
        if old is not None and new_floor < old:
            rows.append((pkg, old))
            kept.append(f"{pkg}: 实测 {pct(covered, total):.1f}% 低于现有底线 {old}%，"
                        f"棘轮保持原值（真下调用 --allow-drop {pkg} 并说明原因）")
            continue
        rows.append((pkg, new_floor))
    header = (
        "# 核心域覆盖率棘轮（scripts/check-core-coverage.py 读取）\n"
        "#\n"
        "# 一行一个核心包：`package | floor_percent`。floor 是**向下取整**的实测值，\n"
        "# 只许涨不许跌 —— 覆盖率下降要么补测试，要么（真有正当理由时）用\n"
        "# `--allow-drop <包名>` 显式下调并在提交信息里签字。\n"
        "#\n"
        "# 为什么按包而不按全仓：全仓平均涨了不代表核心链路被测到，详见脚本文件头。\n"
        "# 刷新：python3 scripts/check-core-coverage.py "
        "--profile user-server/coverage/go-cover.out --refresh\n"
        "#\n"
        "# ─────────────────────────────────────────────────────────────────────\n"
    )
    body = "\n".join(f"{pkg} | {floor}" for pkg, floor in rows)
    bl_path.write_text(header + body + "\n", encoding="utf-8")
    print(f"✅ 基线已刷新：{BASELINE_REL}（{len(rows)} 个核心包）")
    for line in kept:
        print(f"  RATCHET_KEEP: {line}")
    return 0


def fail(msg: str, code: int = 2):
    print(f"❌ {msg}")
    sys.exit(code)


# ── 自检：判据恒绿与判据恒红都长得像"跑过了"，所以两种极性都要有控制 ──────────
def self_check() -> int:
    core = CORE_PREFIXES[0]
    controls: list[tuple[str, bool]] = []

    def pkg(name: str) -> str:
        return f"{core}/{name}"

    # 1 低于底线 ⇒ 红；2 恰好等于底线 ⇒ 绿（不许因为浮点多判红）
    stats = {pkg("a"): (40, 100)}
    f, _ = evaluate(stats=stats, baseline={pkg("a"): 50}, baseline_bad=[]), None
    controls.append(("低于底线 ⇒ BELOW_FLOOR", "BELOW_FLOOR" in {x.code for x in f}))
    f, _ = evaluate(stats={pkg("a"): (50, 100)}, baseline={pkg("a"): 50}, baseline_bad=[]), None
    controls.append(("恰好等于底线 ⇒ 不红（极性对）", not f))
    # 3 高于底线 ⇒ 绿
    f, _ = evaluate(stats={pkg("a"): (80, 100)}, baseline={pkg("a"): 50}, baseline_bad=[]), None
    controls.append(("高于底线 ⇒ 绿", not f))
    # 4 核心包未登记 ⇒ UNREGISTERED
    f, _ = evaluate(stats={pkg("b"): (99, 100)}, baseline={}, baseline_bad=[]), None
    controls.append(("核心包未登记 ⇒ UNREGISTERED", "UNREGISTERED" in {x.code for x in f}))
    # 5 基线有、profile 没有 ⇒ NOT_IN_PROFILE（不是静默当 0%）
    f, _ = evaluate(stats={}, baseline={pkg("c"): 30}, baseline_bad=[], core_exists=lambda p: True), None
    controls.append(("profile 缺包 ⇒ NOT_IN_PROFILE", "NOT_IN_PROFILE" in {x.code for x in f}))
    # 6 包已不存在 ⇒ STALE
    f, _ = evaluate(stats={}, baseline={pkg("gone"): 30}, baseline_bad=[],
                    core_exists=lambda p: False), None
    controls.append(("包已删除 ⇒ STALE", {x.code for x in f} == {"STALE"}))
    # 7 非核心包完全不参与（极性对：判据不许扩到全仓）
    f, _ = evaluate(stats={"hivemtk-user/internal/other/x": (0, 100)}, baseline={}, baseline_bad=[]), None
    controls.append(("非核心包不参与判定", not f))
    # 8 profile 解析：重复语句块不重复计分母、count>0 记覆盖
    prof = "mode: atomic\n" + "\n".join([
        f"{core}/a/x.go:1.1,1.10 5 3",
        f"{core}/a/x.go:2.1,2.10 5 0",
        f"{core}/a/x.go:1.1,1.10 5 7",
    ])
    st = parse_profile(prof)
    controls.append(("profile 去重：2 块共 10 语句、覆盖 5 ⇒ 50%",
                     st.get(pkg("a")) == (5, 10)))

    failed = [n for n, ok in controls if not ok]
    for name, ok in controls:
        print(f"  {'✓' if ok else '✗'} {name}")
    print(f"自检：{len(controls)} 枚控制，失败 {len(failed)} 枚")
    return 0 if not failed else 1


def main() -> int:
    if "--self-check" in sys.argv:
        return self_check()

    profile = REPO_ROOT / DEFAULT_PROFILE
    for i, a in enumerate(sys.argv):
        if a == "--profile" and i + 1 < len(sys.argv):
            profile = Path(sys.argv[i + 1]).resolve()

    if "--refresh" in sys.argv:
        return refresh(profile)

    stats = read_profile(profile)
    bl_path = REPO_ROOT / BASELINE_REL
    if not bl_path.is_file():
        fail(f"基线不存在：{BASELINE_REL}（先跑 --refresh 生成；没有基线就分不清"
             f"「新包没定底线」与「老包退步」，不能绿）")
    baseline, bad = parse_baseline(bl_path.read_text(encoding="utf-8"))

    allow_drop: set[str] = set()
    for i, a in enumerate(sys.argv):
        if a == "--allow-drop" and i + 1 < len(sys.argv):
            allow_drop.add(sys.argv[i + 1])

    findings = []
    for f in evaluate(stats=stats, baseline=baseline, baseline_bad=bad,
                      core_exists=core_exists_in_repo):
        if f.code == "BELOW_FLOOR" and any(f.msg.startswith(p) for p in allow_drop):
            print(f"  DROPPED(已签字): {f.msg}")
            continue
        findings.append(f)

    core_pkgs = [p for p in sorted(stats) if is_core(p)]
    total_cov = sum(stats[p][0] for p in core_pkgs)
    total_stmt = sum(stats[p][1] for p in core_pkgs)
    print(f"扫描面: profile 包 {len(stats)} 个 / 核心包 {len(core_pkgs)} 个 / "
          f"核心域合计 {pct(total_cov, total_stmt):.1f}%（{total_cov}/{total_stmt} 语句，仅参考）")
    print(f"基线: {len(baseline)} 条底线，棘轮只许涨")
    if not findings:
        print("✅ 核心域分档覆盖率门通过：无核心包低于自己的底线，也未出现未登记的新核心包")
        return 0
    for f in findings:
        print(f)
    print("❌ 核心域分档覆盖率门未通过")
    return 1


if __name__ == "__main__":
    sys.exit(main())
