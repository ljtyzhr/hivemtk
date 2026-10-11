#!/usr/bin/env python3
"""参数中心读取点门：种子目录里登记的每个动态参数，要么真有人读，要么自己承认没接线。

背景：`user-server/internal/service/config_param_seeds.go` 的 DefaultParamDefs() 是运维在
「参数中心」页面上看到的清单——页面把每条渲染成可编辑控件，改完就写库并让 ConfigParamService
缓存失效。但门第一次跑出来的实测口径是：114 条里只有 37 条在生产代码里有读取点，其余 77 条
（除 `bridge/polling_max_timeout`、`bridge/polling_default_timeout` 两条已在文案里写明"未接线"）
**改了就是不生效**：没有任何一方读它。这属于"看起来完成、实际从不执行"，且对外（运维）是一句谎话。

判据（任一不成立即 rc=1）：
  1. 每条 (group,key) 要么有读取点（见下方口径），要么条目的 Name/Description 里带
     `未接线` 这个 token；没读取点又不承认 ⇒ UNDECLARED 红。
  2. 带 `未接线` 的条目若已经有了读取点 ⇒ STALE 红（防声明烂掉：接线之后必须撤掉这句）。
  3. 解析对账：正则解出的条目数必须等于 `Key:` 行数，且每个 key 在条目里唯一；
     解不出来就 rc=2 报错，不对"零违规"打勾（枚举空集与全绿在输出上长得一样）。

口径边界（写明本门看不见什么，别把"没扫到"印成"没问题"）：
  - 读取点只认两类字面量形状，且只看 `*_test.go` 与种子文件之外的文件：
      a. 同一行里既有 `"<key>"` 又匹配 `Get(Int|Float|Bool|Duration|String)(`；
      b. `IDENT = "<key>"` 形态的常量，且该 IDENT 出现在某个 `Get*(...)` 行上（同文件跨文件都算）。
  - 经变量拼装/运行时算出的键名不在本门视野内（同 check-env-coverage.py 的边界）。
  - "有读取点"只证明有人调 Get*，不证明读到的值真被用于行为（读进局部变量再丢掉这类形状
    需要人工审查面）；也不核对 group 是否写对——group 写错时 Get 会回落 fallback，
    形状上仍是绿的。

G1/G2（2026-10-11 阶段五「门禁」补）：上面三条是"种子 → 读取点"的**单向**核对
（登记的每一条都得有人读）。反向还有一类故障它结构上抓不到，本门一并补上：

  G1 ORPHAN_READ   代码里有 `GetInt(ctx, "g", "k", …)`，但种子里没有 (g,k) 这一条
                    ⇒ 那个键**读一次 miss 一次、每次都回落 fallback**，而运维在页面上
                    根本看不到这个旋钮。这比"没人读"更隐蔽：读点存在，形状上是绿的。
                    1.8 在 channelgw/ws.go 上栽过一次——种子登记 ws_push_interval，
                    代码写 ws_push_interval_default，每次读都 miss、每次回落兜底，
                    而读取点门禁结构上看不出这类漂移（读取点存在，只是用的别的 key）。
  G2 ORPHAN_GROUP  代码里出现了种子中没有的**组名**（同一 miss 形状，但通常意味着
                    整组都接错了，或组名改了种子忘了跟）。

两轴都只红「新增」，存量由白名单显式豁免（新写一行坏键就得先来这里解释一次，
而不是让整门因为历史包袱常红——常红的门等于没有门）。

执行入口：`make audit`（本地聚合门）。CI 接线待 user-server-ci.yml 上的并行改动回 clean 再补，
补时触发 paths 必须含本脚本与 config_param_seeds.go（否则改判据不触发这道门）。
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
for _i, _a in enumerate(sys.argv):
    if _a == "--repo" and _i + 1 < len(sys.argv):
        REPO_ROOT = Path(sys.argv[_i + 1]).resolve()
MARKER = "未接线"
SEEDS_REL = Path("user-server") / "internal" / "service" / "config_param_seeds.go"
SCAN_REL = Path("user-server")
# 前端消费者白名单：`GET /api/config/runtime` 只暴露这些组的 value。凡是在这份清单里的
# 组，其参数的真实读取点在 user-web（Pinia 的 useRuntimeConfig + 各组件的 str()/num()/
# durSeconds()/bool()），而本门的扫描面只有 user-server —— 所以这些组要么被误判成
# UNDECLARED（前端明明在读），要么为了过门而挂一句「未接线」的谎话。
#
# 解法不是放宽本门，而是让前端自己的注册表成为事实源：这里直接读 controller 里的
# runtimePublicGroups 字面量集合。新增组必须同时出现在两处，否则本条解析失败、门红
# （宁可红，也不要一个悄悄失效的豁免）。
FRONTEND_RT_REL = Path("user-server") / "internal" / "controller" / "config_param_runtime.go"
GETCALL = re.compile(r'Get(?:Int|Float|Bool|Duration|String)\(')
KEY_LINE = re.compile(r'Key:\s*"([a-z0-9_]+)"')
# 条目头：每个条目的固定前缀是 `{Group: "g", Key: "k", Name: "n"`，其余字段顺序不定
# （`38ea7489` 那两条 lead 条目把 DefaultValue/ValueType/Category 写在 Description 之前且整条一行，
#  按"Name 之后必须紧跟换行 + Description"匹配的旧写法解不出来 ⇒ 枚举源看不见它们，
#  而它看不见的那几条照样会被算进"全绿"）。
# 因此这里只锚条目头，Description 在该条目到下一个条目头之间的片段里找。
ENTRY_HEAD = re.compile(r'\{Group:\s*"([a-z_]+)",\s*Key:\s*"([a-z0-9_]+)",\s*Name:\s*"([^"]*)"')
ENTRY_DESC = re.compile(r'Description:\s*"([^"]*)"')
CONST_ASSIGN = re.compile(r'\b([A-Za-z_]\w*)\s*=\s*"([a-z0-9_]+)"')


def parse_entries(text: str) -> list[tuple[str, str, str, str]]:
    """列出 (group, key, name, description)。

    没有 Description 的条目返回空串：它照样要过判据 1（没读取点又不承认 ⇒ UNDECLARED），
    不能因为"读不出文案"就绕过分诊。
    """
    heads = list(ENTRY_HEAD.finditer(text))
    out: list[tuple[str, str, str, str]] = []
    for i, m in enumerate(heads):
        end = heads[i + 1].start() if i + 1 < len(heads) else len(text)
        d = ENTRY_DESC.search(text[m.end():end])
        out.append((m.group(1), m.group(2), m.group(3), d.group(1) if d else ""))
    return out


def fail(msg: str, code: int = 2) -> "NoReturn":  # type: ignore[name-defined]
    print(f"❌ {msg}")
    sys.exit(code)


def main() -> int:
    seeds = REPO_ROOT / SEEDS_REL
    scan_root = REPO_ROOT / SCAN_REL
    if not seeds.is_file():
        fail(f"种子文件不存在: {seeds}（本门没有可核对的对象，不是通过）")
    seed_text = seeds.read_text(encoding="utf-8")

    key_rows = KEY_LINE.findall(seed_text)
    entries = parse_entries(seed_text)
    if not key_rows:
        fail("种子文件里一条 Key 都没解出：枚举源坏了，不能据此说「没有违规」")
    if len(entries) != len(key_rows):
        fail(f"条目解析对账失败: 解出条目 {len(entries)} 条，`Key:` 行 {len(key_rows)} 行"
             "（条目形状变了，本门的枚举源就瞎了）")
    seen: set[tuple[str, str]] = set()
    for g, k, _n, _d in entries:
        if (g, k) in seen:
            fail(f"种子里有重复条目 group={g} key={k}（本门按条目定位，重复会让判据落到错的条目上）")
        seen.add((g, k))

    # 收集读取点素材：非测试、非种子文件的逐行 Go 文本
    files = [p for p in scan_root.rglob("*.go")
             if not p.name.endswith("_test.go") and p.resolve() != seeds.resolve()]
    if not files:
        fail(f"{scan_root} 下没解到任何非测试 Go 文件：环境或路径不对，不作绿灯判定")
    get_lines: list[str] = []
    key_lines: list[tuple[str, str]] = []          # (含 "key" 的行, 该行文本)
    const_map: dict[str, list[str]] = {}           # key -> 承载它的标识符
    for p in files:
        try:
            text = p.read_text(encoding="utf-8", errors="replace")
        except OSError as e:
            fail(f"读取 {p} 失败: {e}")
        for line in text.splitlines():
            if GETCALL.search(line):
                get_lines.append(line)
            if '"' in line:
                key_lines.append((line, str(p)))
            for m in CONST_ASSIGN.finditer(line):
                const_map.setdefault(m.group(2), []).append(m.group(1))

    joined_gets = "\n".join(get_lines)

    # G1/G2 素材：从每个 Get*( 调用里解出 (group, key) 字面量对。
    # 形状：GetInt(<ctx>, "<group>", "<key>"[, fallback])  /  GetInt(<ctx>, GroupConst, "<key>")
    # 两种都收：第二种的 group 走 CONST 解析（见下方 group_const）。
    read_pairs: set[tuple[str, str]] = set()
    read_groups: set[str] = set()
    # ctx 实参形状很多样：`ctx` / `bg` / `context.Background()` /
    # `service.GlobalConfigParam()`（这是调用对象不是 ctx，但它在 Get* 之前）。
    # 这里只要求"第一个实参不是紧跟的字符串字面量"，用非贪婪的任意非逗号片段兜住。
    GETCALL_PAIR = re.compile(
        r'Get(?:Int|Float|Bool|Duration|String)\(\s*(?:"([a-z0-9_]+)"|([A-Za-z_]\w*))\s*,\s*"([a-z0-9_]+)"')
    group_const: dict[str, str] = {}
    for line in get_lines:
        for m in GETCALL_PAIR.finditer(line):
            g_lit, g_ident, k = m.group(1), m.group(2), m.group(3)
            if g_lit:
                read_pairs.add((g_lit, k))
                read_groups.add(g_lit)
            # g_ident 形态留到下方 group_const 建好后回填
    # `ParamGroupXxx = "xxx"` 形态的组名常量
    for line in joined_gets.split("\n"):
        for m in re.finditer(r'\b([A-Za-z_]\w*)\s*=\s*"([a-z0-9_]+)"', line):
            group_const.setdefault(m.group(1), m.group(2))
    # 全文件扫一遍组名常量（不限于 Get* 行，常量通常在别处声明）
    for fpath in files:
        try:
            ftext = fpath.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        for m in re.finditer(r'\b([A-Za-z_]\w*)\s*=\s*"([a-z0-9_]+)"', ftext):
            group_const.setdefault(m.group(1), m.group(2))
    for line in get_lines:
        for m in GETCALL_PAIR.finditer(line):
            g_ident, k = m.group(2), m.group(3)
            if g_ident and g_ident in group_const:
                read_pairs.add((group_const[g_ident], k))
                read_groups.add(group_const[g_ident])

    # G1/G2 白名单：存量孤儿显式豁免。新增一行坏键必须先在这里解释一次。
    # 形状：{"group.key": "理由"}。
    orphan_allowlist: dict[str, str] = {}

    # 前端消费者组：从 controller/config_param_runtime.go 的 runtimePublicGroups map
    # 字面量里解出键名。解不出来就 rc=1（不是 rc=2 的"枚举源坏了"：这是"豁免清单
    # 与实现不一致"，属于判据失准，必须让人看见）。
    if not FRONTEND_RT_REL.is_file():
        fail(f"前端只读端点文件不存在: {FRONTEND_RT_REL}"
             "（它承载 runtimePublicGroups 白名单，本门靠它判定前端消费组）", 1)
    rt_text = FRONTEND_RT_REL.read_text(encoding="utf-8", errors="replace")
    # 注意 `[^}]*` 会在第一个值的 `{}` 处截断（map[string]struct{} 的每个值就是一对
    # 空花括号），只能解出第一组键。改为解到「末尾的 `}` 且其后紧跟换行或 EOF」，
    # 即在 map 字面量内部允许任意 `{}`。
    rt_block = re.search(r"var runtimePublicGroups\s*=\s*map\[string\]struct\{}\{(.*?)\n\}",
                         rt_text, re.S)
    if not rt_block:
        fail("config_param_runtime.go 里解不出 runtimePublicGroups 的 map 字面量"
             "（形状变了，前端口径失准）", 1)
    frontend_groups = set(re.findall(r'"([a-z0-9_]+)"\s*:', rt_block.group(1)))
    if not frontend_groups:
        fail("runtimePublicGroups 解出来是空集合", 1)
    # 组内排除清单：这些键即使所属组在白名单里也不下发给前端。
    rt_excl = re.search(r"var runtimePublicKeys\s*=\s*map\[string\]struct\{}\{(.*?)\n\}",
                        rt_text, re.S)
    frontend_excluded = set(re.findall(r'"([a-z0-9_]+\.[a-z0-9_]+)"\s*:', rt_excl.group(1))) \
        if rt_excl else set()

    wired: list[str] = []
    unwired_marked: list[str] = []
    undeclared: list[str] = []
    stale: list[str] = []
    for g, k, name, desc in entries:
        direct = any(f'"{k}"' in line and GETCALL.search(line) for line, _p in key_lines)
        # 标识符按整词匹配：子串会把 `Key` 这类短名算成别的词的一部分（读到就判绿）。
        via_const = any(re.search(rf'\b{re.escape(ident)}\b', joined_gets)
                        for ident in const_map.get(k, []))
        # 前端消费：读取点在 user-web，由 runtimePublicGroups/Keys 作证。
        # 判定按 group.key 粒度：组在白名单里还不够，键不能在排除清单里——
        # 否则「组整体给前端、个别键后端没人读」会被整组判成已接线，
        # 而那些键的 Description 里明写着「当前不生效」。
        coord = f"{g}.{k}"
        via_frontend = (g in frontend_groups) and (coord not in frontend_excluded)
        has_read = direct or via_const or via_frontend
        declared = MARKER in name or MARKER in desc
        coord = f"{g}.{k}"
        # 四类互斥：一条只能落进一格。曾经把 STALE 同时记进"有读取点"，
        # 于是四类之和 > 条目数，对账格把门自己判红了。
        if has_read and declared:
            stale.append(coord)
        elif has_read:
            wired.append(coord)
        elif declared:
            unwired_marked.append(coord)
        else:
            undeclared.append(coord)

    print(f"扫描面: Go 文件（非测试、非种子）{len(files)} 个 / Get* 行 {len(get_lines)} 行 / "
          f"种子条目 {len(entries)} 条 / 前端消费组 {len(frontend_groups)} 个（排除 "
          f"{len(frontend_excluded)} 键）")
    print(f"读数: 有读取点且未挂标注={len(wired)}  未接线且已声明={len(unwired_marked)}  "
          f"未接线但未声明={len(undeclared)}  声明过期={len(stale)}")
    total = len(wired) + len(unwired_marked) + len(undeclared) + len(stale)
    if total != len(entries):
        fail(f"分类对账失败: 四类之和 {total} != 条目数 {len(entries)}（有条目既没被分类也没被丢弃，"
             "说明判据漏了分支）")

    for coord in undeclared:
        print(f"  UNDECLARED: {coord} 没有读取点，条目文案里也没写「{MARKER}」"
              "——运维在页面上改它是不生效的")
    for coord in stale:
        print(f"  STALE: {coord} 已声明「{MARKER}」但代码里已有读取点，请把那句撤掉")

    # ── G1/G2：反向核对（读取点 → 种子）──
    # 注意 entries 是 (group, key, name, desc) 四元组，取前两列比。
    seeded = {(g, k) for g, k, _n, _d in entries}
    seeded_groups = {g for g, _k, _n, _d in entries}
    orphan_read = sorted(f"{g}.{k}" for g, k in read_pairs
                        if (g, k) not in seeded and f"{g}.{k}" not in orphan_allowlist)
    orphan_group = sorted(read_groups - seeded_groups - {""})
    for coord in orphan_read:
        print(f"  G1 ORPHAN_READ: {coord} 代码里有读取点但种子没登记"
              "——每次读都 miss、每次都回落 fallback，运维在页面上看不到这个旋钮")
    for g in orphan_group:
        print(f"  G2 ORPHAN_GROUP: {g} 代码里出现了种子中没有的组名"
              "——整组都接错了，或组名改了种子忘了跟")

    if undeclared or stale or orphan_read or orphan_group:
        print("❌ 参数中心读取点门未通过")
        return 1
    print("✅ 参数中心读取点门通过：每条要么有人读，要么自己承认没接线；"
          "且没有孤儿读取点（读了种子没登记的键）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
