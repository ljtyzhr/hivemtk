#!/usr/bin/env python3
"""G4：新增的「可调旋钮」硬编码必须入库（只卡新增，不卡存量）。

为什么需要它：`config_params` 建起来之后，最容易退化的不是「不接旧账」而是
**继续往里加新的硬编码**。3.1 五批接了 54 条，但同一个模式还在被复制——
下一个人加一个新的阈值常量时，没有任何一道门会问他「这个要不要入库」。

判据（rc=1）：
  对 `user-server/internal` 与 `cmd` 下**非测试、非种子**的 Go 文件，找出形如

      XxxThreshold = 30
      XxxMaxSize   = 10 * 1024 * 1024
      XxxTTL       = 5 * time.Second

  的**命名常量**（名字带 Threshold/Limit/Max/Min/TTL/Timeout/Interval/Window/
  Size/Count/Concurrency/Retries/Rate/Weight/Percent/Seconds/Days/Hours 之一），
  且它**不在基线清单**里 ⇒ NEW_TUNABLE 红。

  基线（`docs/HARDCODE_INVENTORY.md` §2.1 A 桶 / hardcode-sweep 的 TUNABLE 归类）
  是显式的：存量照常编译，但要动它就得先入库。
  新增不在巡检面——写一个新常量而不入库，这道门当场红。

**为什么按名字判而不是按值判**：值判不出语义（`30` 可以是端口、版本号、协议 op 码，
也可以是阈值）。名字带这些词缀的常量在仓内有稳定语义，且误报可以由基线消化。
代价是改个名就能绕开——所以旁边配 G5 的 keystyle 门与人工审查面，不指望一道门包打天下。

口径边界（别把"没扫到"印成"没问题"）：
  - 只看命名常量赋值。函数内的裸字面量（`f(30)`）不在本门视野——那属于 INLINE_NUM，
    由 hardcode_sweep.py 体检，治理结论是"不治理"（见 HARDCODE_INVENTORY §6）。
  - 协议常量（错误码、端口、状态枚举、版本号）由 EXEMPT_NAME 正则豁免，命中仍打印
    但不计红。
  - `Default*` 前缀是**参数中心兜底值**的约定标记，同样豁免（见 DEFAULT_PREFIX）。
    它们不是新的硬编码旋钮，而是 provider 注入缺失时的 fallback；判红等于让
    "接线"这个动作本身变成违规。它们受 G3 与 readpoints 门约束。
  - 基线清单是**文件级**的：整份 hardcode-sweep-classified.json 的 TUNABLE 集合。
    没有那份文件时本门退 2（不是绿）。
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
for _i, _a in enumerate(sys.argv):
    if _a == "--repo" and _i + 1 < len(sys.argv):
        REPO_ROOT = Path(sys.argv[_i + 1]).resolve()

CLASSIFIED_REL = Path("hardcode-sweep") / "hardcode-sweep-classified.json"
BASELINE_REL = Path("scripts") / "hardcode-tunable.baseline"
GO_ROOTS = [Path("user-server") / "internal", Path("user-server") / "cmd"]

# 跳过目录按**相对扫描根**的路径段判定，不看绝对路径。
# 踩过的坑：原先写 `seg in p.parts`，而 `p` 是 rglob 出来的绝对路径——只要
# 仓库被检出到任何含 `tmp` / `test` / `migrations` 段的位置（本仓的干净 worktree
# 验证恰好就在 /tmp 下），**每一个文件都会被跳过**，门扫出 0 条常量。
# 表现是 rc=2 而不是恒绿（本门拒绝冒充绿），但真因在路径判定，修在这里。
SKIP_SEGMENTS = {"cmd/seed", "vendor", "migrations", "test", "tests", "tmp", "backups"}

TUNABLE_SUFFIX = re.compile(
    r"(Threshold|Limit|Max|Min|TTL|Timeout|Interval|Window|Size|Count|Concurrency|"
    r"Retries|Rate|Weight|Percent|Seconds|Days|Hours|Cap|Ceiling|Floor|Batch)", re.I)
EXEMPT_NAME = re.compile(
    r"^(Code|Err|Status|SpanIDHexLen|TraceIDHexLen|RDays|Version|Port|DefaultPort|"
    r"ImplicitTLSPort|AGENT_RUNTIME_BUS|SSL|DSN|MaxOpenConns|MaxIdleConns|"
    r"HNSW|Dimension|TopLogprobs|SourceDim|BGEM3)", re.I)
# `Default*` 前缀是参数中心兜底值的约定标记：这些常量名字里带 Default，是因为
# 它们在 provider 注入缺失时当 fallback 用（3.1 五批统一留下的形态）。
# 它们**已经**是参数中心的一份子，不是新的硬编码旋钮——判红等于让"接线"这个动作
# 本身变成违规。仍受 G3（默认值与种子一致）与 readpoints 门约束。
DEFAULT_PREFIX = re.compile(r"^Default")
# 语义上限/下限：不是运维可调的旋钮，而是某个值域的数学边界。
#   - OpportunityWinProbabilityMax = 1.0：赢率是概率，上限恒为 1，调它没有意义
#     （ltc.config 的 win_probability 也按 [0,1] 校验）。
# 判据：这类常量的值只可能取「值域的极值」，改了不会让系统行为变"可调"，
# 只会让同一份数据在边界两侧给出矛盾的结论。
SEMANTIC_BOUND = re.compile(r"(ProbabilityMax|ProbabilityMin|RatioMax|RatioMin)$")
CONST_DECL = re.compile(r'^\s*([A-Z]\w*)\s*=\s*(\d[\w.]*|\d+\s*\*\s*[\w.]+)\s*(?://.*)?$', re.M)
# 兜底实参位：GetInt(ctx, "bridge", "ingest_max_messages", HTTPIngestMaxMessages)
# 取最后一个逗号分段。实参里允许一层括号（GlobalConfigParam() 之类），
# 但不允许再嵌括号——真出现嵌套时这条正则不匹配，退化成"未接线"判红，不会漏放。
GET_CALL = re.compile(
    r"Get(?:Int|Float|Bool|Duration|String)\(((?:[^()\n]|\([^()]*\))*)\)", re.M)
FALLBACK_IDENT = re.compile(r"^[A-Za-z_]\w*$")
# 直连范式的哨兵写法：GetInt(ctx, group, key, 0) 之后回落到常量。
#   if v := cp.GetInt(ctx, "memory", "l1_window_size", 0); v > 0 { return v }
#   return L1WindowSize
# 兜底实参是字面量 0（"没读到值"哨兵），真正的兜底常量在随后的 return/赋值上。
# 取调用结束后的 5 行——足够覆盖上面这个两行形态，又不会跨到别的函数。
SENTINEL_TAIL = re.compile(r"(?:return\s+|:=\s*|=\s*)([A-Z]\w*)\b")
SENTINEL_LOOKAHEAD_LINES = 5


def collect_fallback_idents(text: str) -> set[str]:
    """扫出所有被当作参数中心兜底值传进去的常量名。

    这条判据比"常量名和种子 key 对得上"准确得多：接线时保留原常量名
    （避免把参数化放大成一片 diff），所以名字往往与 key 毫无关系——
    `HTTPIngestMaxMessages` 的种子 key 是 `bridge.ingest_max_messages`，
    `L1WindowSize` 的是 `memory.l1_window_size`。按名字配对只能靠一张
    人工维护的映射表，而映射表必然和代码漂移。实参位置不会漂移。

    同时覆盖两种接线范式（3.1a 定的判据）：
      1) provider seam：常量直接出现在 Get* 的兜底实参位
      2) 直连：Get* 兜底传 0，函数体随后回落到常量
    """
    out: set[str] = set()
    lines = text.splitlines()
    for m in GET_CALL.finditer(text):
        args = m.group(1)
        parts = args.split(",")
        if len(parts) < 3:
            # 少于三个实参不是本门关心的形态（(ctx, group, key) 这种没有兜底位）
            continue
        last = parts[-1].strip()
        if FALLBACK_IDENT.match(last):
            out.add(last)
            continue
        if last != "0":
            continue
        # 哨兵范式：从调用结束处往后找几行，看有没有回落到某个大写常量
        start_line = text.count("\n", 0, m.end())
        tail = lines[start_line:start_line + SENTINEL_LOOKAHEAD_LINES]
        for ln in tail:
            if "func " in ln:
                break
            sm = SENTINEL_TAIL.search(ln)
            if sm:
                out.add(sm.group(1))
    return out


def fail(msg: str, code: int = 2):
    print(f"❌ {msg}")
    sys.exit(code)


def load_baseline() -> set[str]:
    """读 `scripts/hardcode-tunable.baseline`：存量 TUNABLE 常量名，一行一个。

    为什么自己落一份而不直接读 hardcode-sweep-classified.json：
      1) 判据文件要和门脚本一起进 CI 触发面（`scripts/check-ci-gate-paths.py`
         的第 2 条）。那份 classified 是 210KB 的可再生产物，跟门没有稳定的
         一对一关系；独立成 `.baseline` 之后形状与其他五道门一致，
         也符合本仓 `*.baseline` 的既有约定。
      2) 本门只需要 286 个名字，不需要 file/line/value 等其余字段。
      3) 基线与生成物解耦后，重新跑 `hardcode_classify.py` 不会顺手把
         门的判据改掉——刷基线必须是人的显式动作。
    刷新：`python3 scripts/check-hardcode-tunable.py --refresh-baseline`
    （需要磁盘上存在 hardcode-sweep-classified.json）。
    """
    p = REPO_ROOT / BASELINE_REL
    if not p.is_file():
        fail(f"基线文件不存在: {p}（跑本脚本 --refresh-baseline 从 "
             f"{CLASSIFIED_REL} 生成；没有基线本门无法区分「新增」与「存量」，不能绿）")
    out: set[str] = set()
    for ln in p.read_text(encoding="utf-8").splitlines():
        name = ln.strip()
        if not name or name.startswith("#"):
            continue
        out.add(name)
    return out


def refresh_baseline() -> int:
    classified = REPO_ROOT / CLASSIFIED_REL
    if not classified.is_file():
        fail(f"刷基线需要 {CLASSIFIED_REL}（跑 scripts/hardcode_classify.py 生成）")
    data = json.loads(classified.read_text(encoding="utf-8"))
    names = sorted({r.get("name") for r in (data.get("rows") or [])
                    if r.get("bucket") == "TUNABLE" and r.get("name")})
    header = (
        "# 存量「可调旋钮」常量名基线（scripts/check-hardcode-tunable.py 读取）。\n"
        "# 一行一个 Go 常量名。它代表的是「存量已经数过、判定为存量待收口」，\n"
        "# 不是「这样写是对的」——本表只挡新增，不发合格证。\n"
        f"# 来源：{CLASSIFIED_REL} 的 TUNABLE 归类；刷新命令：\n"
        "#   python3 scripts/check-hardcode-tunable.py --refresh-baseline\n"
        "# 刷新前先确认这次是「把该收口的收口了」而不是「想把这笔从门里摘出去」。\n"
    )
    (REPO_ROOT / BASELINE_REL).write_text(
        header + "\n".join(names) + "\n", encoding="utf-8")
    print(f"✅ 基线已刷新: {BASELINE_REL}（{len(names)} 个存量常量名）")
    return 0


def scan_go() -> tuple[list[tuple[str, str]], set[str]]:
    out: list[tuple[str, str]] = []
    fallback: set[str] = set()
    for root in GO_ROOTS:
        base = REPO_ROOT / root
        if not base.is_dir():
            continue
        for p in base.rglob("*.go"):
            if p.name.endswith("_test.go"):
                continue
            # 跳过目录只看**相对扫描根**的路径段：SKIP_SEGMENTS 里带斜杠的
            # 是多段前缀（cmd/seed），纯名字的是单段。
            rel_parts = p.relative_to(base).parts
            skipped = any("/".join(rel_parts[:i + 1]) in SKIP_SEGMENTS
                          for i in range(len(rel_parts)))
            skipped = skipped or any(seg in SKIP_SEGMENTS
                                     for seg in rel_parts if "/" not in seg)
            if skipped:
                continue
            try:
                t = p.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue
            rel = str(p.relative_to(REPO_ROOT))
            for m in CONST_DECL.finditer(t):
                out.append((rel, m.group(1)))
            fallback |= collect_fallback_idents(t)
    return out, fallback


def main() -> int:
    if "--refresh-baseline" in sys.argv:
        return refresh_baseline()
    baseline = load_baseline()
    if not baseline:
        fail("基线 TUNABLE 集合为空")
    decls, fallback = scan_go()
    if not decls:
        fail("一个 Go 常量声明都没扫到：环境或路径不对")

    wired: list[tuple[str, str]] = []
    new: list[tuple[str, str]] = []
    exempt: list[tuple[str, str]] = []
    for path, name in decls:
        if name in baseline:
            continue
        if EXEMPT_NAME.match(name):
            exempt.append((path, name))
            continue
        if DEFAULT_PREFIX.match(name):
            exempt.append((path, name))
            continue
        if SEMANTIC_BOUND.search(name):
            exempt.append((path, name))
            continue
        if name in fallback:
            wired.append((path, name))
            continue
        if TUNABLE_SUFFIX.search(name):
            new.append((path, name))

    print(f"扫描面: Go 常量声明 {len(decls)} 个 / 基线 TUNABLE {len(baseline)} 个")
    print(f"读数: 新增未入库={len(new)}  已作参数中心兜底={len(wired)}  "
          f"豁免（协议类/参数中心兜底前缀）={len(exempt)}")
    for path, name in wired:
        print(f"  WIRED_FALLBACK: {path}: {name} 已被 Get*() 当兜底值传入参数中心")
    for path, name in new:
        print(f"  NEW_TUNABLE: {path}: {name} 名字像可调旋钮但既不在基线也不在参数中心")
    if new:
        print("❌ 新增可调旋钮门未通过：入库（config_params）或加入基线并说明为什么不用入库")
        return 1
    print("✅ 新增可调旋钮门通过：没有绕过参数中心的新旋钮常量")
    return 0


if __name__ == "__main__":
    sys.exit(main())
