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

第五类「已下架」（2026-10-12 阶段六补）：条目标了 `Deprecated: true` 之后，它既不是
wired（管理端 List 默认过滤掉、runtime 也不下发）也不是 UNDECLARED（运维在页面上
根本碰不到它）——拿前两类去套会得出两处都错的结论。所以它单列，并自带两条判据：

  RETIRED_BUT_WIRED   已下架却已经接上了。管理端默认过滤下架项 ⇒ 运维看不见也改不了这个
                      旋钮，而代码真在读它的值。参数被改过、行为跟着变，但页面上找不到
                      它在哪改的 —— 本门要抓的就是这一类，它比 UNDECLARED 更难查。
  DEPRECATED_NO_REASON 已下架却没写 DeprecatedReason。页面上只剩一个「已下架」标签，
                      用户既不知道为什么，也无从知道将来怎么接回来。

下架与 `未接线` 的分工：`未接线` 是「我承认这个旋钮改了不生效」（过渡态，允许存在）；
`Deprecated` 是「我不再对外承诺这个旋钮」（终态，管理端不可见、写入被拒）。
两者都留着就是债 —— 所以阶段六把 11 条过渡态一次性推进到终态，本门的
「未接线且已声明」读数因此归零。

执行入口：`make audit`（本地聚合门）+ `.github/workflows/user-server-ci.yml`。
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
# 下架标记：`Deprecated: true`。它把一条参数从「可配置旋钮」降级成「历史记录」，
# 管理端默认不返回、runtime 不下发、写入被 ErrParamDeprecated 拒掉。
ENTRY_DEPRECATED = re.compile(r'Deprecated:\s*true')
ENTRY_DEPRECATED_REASON = re.compile(r'DeprecatedReason:\s*"([^"]*)"')
CONST_ASSIGN = re.compile(r'\b([A-Za-z_]\w*)\s*=\s*"([a-z0-9_]+)"')

WEB_SRC_REL = Path("user-web") / "src"
# 前端读取口的调用形状：rt*(group, key, 兜底实参)，四个函数都定义在
# user-web/src/utils/runtimeConfig.js（值来自 GET /api/config/runtime，缺失即回落第 3 个实参）。
# 第 3 个实参**不参与**「这条键有没有人读」的判断（写成字面量也算读），
# 只是要单独取出它是不是一个常量名——清单那边判「这个常量接完了没有」用的就是它。
# 已知取舍：第 3 个实参里再套括号（`rtNum('g','k', Math.max(1, X))`）时 [^)] 会在内层
# 括号处截断、整条不匹配 ⇒ 少记一个读取点，方向是**向红偏置**（门会问起这条键），不会假绿。
WEB_READPOINT = re.compile(
    r"""\brt(?:Num|Dur|Bool|Str)\(\s*(['"])([^'"]+)\1\s*,\s*(['"])([^'"]+)\3\s*,\s*([^)]*)\)"""
)
# JS 的字面量关键字长得和标识符一样，但它们是「把值写在调用点」，不是常量名。
WEB_LITERAL_WORDS = {"true", "false", "null", "undefined", "NaN"}
WEB_IDENT = re.compile(r"^[A-Za-z_$][\w$]*$")
# 测试夹具里的 rt* 调用不是生产读取点（与 Go 侧排 _test.go 同一口径）。
WEB_SKIP_SEGMENTS = {"node_modules", "dist", "__tests__", "tests", "test", "coverage", "public"}


def collect_frontend_readpoints(root: Path) -> dict[str, set[str]]:
    """返回 {group.key: 该键作为兜底实参出现的**常量名**集合（字面量兜底则集合为空）。

    为什么不再拿 runtimePublicGroups 当「前端在读」的证据：白名单只证明**服务端愿意下发**，
    不证明**前端真的读了**。实测两条 `frontend_ui` 键（toast_duration_ms、
    websocket_reconnect_max_wait_ms）在 user-web 全树零调用，却因所属组在白名单里
    被本门算成「有读取点」——正是本门在 Go 侧要抓的那类「登记了、页面上能改、改了没人读」，
    只是换到了前端这一臂上。白名单仍然有用，但它的角色变成另一端的对账（见 not_dispatched）。
    """
    out: dict[str, set[str]] = {}
    for path in sorted(root.rglob("*")):
        if path.suffix not in (".js", ".vue") or not path.is_file():
            continue
        if WEB_SKIP_SEGMENTS.intersection(path.relative_to(root).parts):
            continue
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        for m in WEB_READPOINT.finditer(text):
            coord = f"{m.group(2)}.{m.group(4)}"
            names = out.setdefault(coord, set())
            fallback = m.group(5).strip()
            if WEB_IDENT.match(fallback) and fallback not in WEB_LITERAL_WORDS:
                names.add(fallback)
    return out



def parse_entries(text: str) -> list[tuple[str, str, str, str, bool, str]]:
    """列出 (group, key, name, description, deprecated, deprecated_reason)。

    没有 Description 的条目返回空串：它照样要过判据 1（没读取点又不承认 ⇒ UNDECLARED），
    不能因为"读不出文案"就绕过分诊。

    `deprecated` / `deprecated_reason` 的取法与 Description 同理：在该条目到下一个
    条目头之间的片段里找。不在 parse 里做「有 reason 没 flag」这类一致性判定 ——
    那是需要出红字的判据，混在解析里会变成解析失败（rc=2），两种红含义不同。
    """
    heads = list(ENTRY_HEAD.finditer(text))
    out: list[tuple[str, str, str, str, bool, str]] = []
    for i, m in enumerate(heads):
        end = heads[i + 1].start() if i + 1 < len(heads) else len(text)
        span = text[m.end():end]
        d = ENTRY_DESC.search(span)
        r = ENTRY_DEPRECATED_REASON.search(span)
        out.append((m.group(1), m.group(2), m.group(3),
                    d.group(1) if d else "",
                    ENTRY_DEPRECATED.search(span) is not None,
                    r.group(1) if r else ""))
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
    for g, k, _n, _d, _dep, _dep_r in entries:
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
    # 必须按 --repo 解析：写成相对 CWD 的话，用例夹具（--repo 指临时树、CWD 仍在真仓库）
    # 会读到真仓库的白名单——夹具里 demo 组的判定实际用的是 frontend_ws 那张表，
    # 「夹具满足判据前置」这条就不成立了。
    rt_file = REPO_ROOT / FRONTEND_RT_REL
    if not rt_file.is_file():
        fail(f"前端只读端点文件不存在: {rt_file}"
             "（它承载 runtimePublicGroups 白名单，本门靠它判定前端消费组）", 1)
    rt_text = rt_file.read_text(encoding="utf-8", errors="replace")
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
    # 跨组单独放行的键：所属组不在白名单，但这一个键下发给前端。
    # 解不出来时按空集处理是对的——这个清单是可选的，缺它只是回到「只有组级白名单」，
    # 而前端如果真在读一个跨组键，下面的 not_dispatched 会照旧报红，不会假绿。
    rt_cross = re.search(r"var runtimeCrossGroupKeys\s*=\s*map\[string\]struct\{}\{(.*?)\n\}",
                         rt_text, re.S)
    frontend_cross = set(re.findall(r'"([a-z0-9_]+\.[a-z0-9_]+)"\s*:', rt_cross.group(1))) \
        if rt_cross else set()

    # 前端真实读取点：扫 user-web/src 的 rt*(group, key, 兜底) 调用。
    # 目录不存在 ⇒ rc=1（与上面白名单同一口径：这是判据失准，不是内容红）。
    web_src = REPO_ROOT / WEB_SRC_REL
    if not web_src.is_dir():
        fail(f"前端扫描面不存在: {web_src}"
             "（本门靠它判定前端是否真的读了某条参数，缺目录会让前端组全部误判成没读取点）", 1)
    frontend_reads = collect_frontend_readpoints(web_src)

    wired: list[str] = []
    unwired_marked: list[str] = []
    undeclared: list[str] = []
    stale: list[str] = []
    retired: list[str] = []
    retired_but_wired: list[str] = []
    reason_missing: list[str] = []
    read_not_dispatched: list[str] = []
    for g, k, name, desc, deprecated, dep_reason in entries:
        direct = any(f'"{k}"' in line and GETCALL.search(line) for line, _p in key_lines)
        # 标识符按整词匹配：子串会把 `Key` 这类短名算成别的词的一部分（读到就判绿）。
        via_const = any(re.search(rf'\b{re.escape(ident)}\b', joined_gets)
                        for ident in const_map.get(k, []))
        # 前端消费：只认 user-web 里真实出现的 rt* 调用（group.key 粒度）。
        coord = f"{g}.{k}"
        via_frontend = coord in frontend_reads
        has_read = direct or via_const or via_frontend
        declared = MARKER in name or MARKER in desc
        # 五类互斥：一条只能落进一格。曾经把 STALE 同时记进"有读取点"，
        # 于是各类之和 > 条目数，对账格把门自己判红了。
        #
        # 下架（Deprecated）先于其他判据分诊，因为它是「这条旋钮不再是对外承诺」，
        # 不属于「接线做得好不好」这条线。下架条目既不是 wired（管理端看不到它）也
        # 不是 UNDECLARED（运维在页面上根本碰不到它），拿前两类去套都会得出错误结论。
        if deprecated:
            if not dep_reason.strip():
                reason_missing.append(coord)
            if has_read:
                # 接上了却还挂着下架标记：管理端 List 默认过滤 deprecated=true，
                # 运维看不到这个旋钮，改不了；代码却真在读它的值。这是最难查的一类 ——
                # 参数被改过、行为跟着变，但页面上找不到它在哪改的。
                retired_but_wired.append(coord)
            else:
                retired.append(coord)
        elif has_read and declared:
            stale.append(coord)
        elif has_read:
            wired.append(coord)
        elif declared:
            unwired_marked.append(coord)
        else:
            undeclared.append(coord)

    print(f"扫描面: Go 文件（非测试、非种子）{len(files)} 个 / Get* 行 {len(get_lines)} 行 / "
          f"种子条目 {len(entries)} 条 / 前端消费组 {len(frontend_groups)} 个（排除 "
          f"{len(frontend_excluded)} 键）/ 前端真实读取点 {len(frontend_reads)} 个 group.key")
    print(f"读数: 有读取点且未挂标注={len(wired)}  未接线且已声明={len(unwired_marked)}  "
          f"未接线但未声明={len(undeclared)}  声明过期={len(stale)}  已下架={len(retired)}"
          f"（其中接上了却没摘下架标记={len(retired_but_wired)}）")
    total = (len(wired) + len(unwired_marked) + len(undeclared) + len(stale)
             + len(retired) + len(retired_but_wired))
    if total != len(entries):
        fail(f"分类对账失败: 各类之和 {total} != 条目数 {len(entries)}（有条目既没被分类也没被丢弃，"
             "说明判据漏了分支）")

    for coord in undeclared:
        print(f"  UNDECLARED: {coord} 没有读取点，条目文案里也没写「{MARKER}」"
              "——运维在页面上改它是不生效的")
    for coord in stale:
        print(f"  STALE: {coord} 已声明「{MARKER}」但代码里已有读取点，请把那句撤掉")
    for coord in retired_but_wired:
        print(f"  RETIRED_BUT_WIRED: {coord} 已下架但代码里已有读取点——"
              "管理端默认过滤下架项，运维看不见也改不了这个旋钮，请摘掉下架标记")
    for coord in reason_missing:
        print(f"  DEPRECATED_NO_REASON: {coord} 标了下架却没写 DeprecatedReason——"
              "页面上会只剩一个「已下架」标签，用户无从知道为什么，也无从知道将来怎么接回来")

    # ── G1/G2：反向核对（读取点 → 种子）──
    # 注意 entries 是 (group, key, name, desc) 四元组，取前两列比。
    seeded = {(g, k) for g, k, _n, _d, _dep, _dr in entries}
    seeded_groups = {g for g, _k, _n, _d, _dep, _dr in entries}
    orphan_read = sorted(f"{g}.{k}" for g, k in read_pairs
                        if (g, k) not in seeded and f"{g}.{k}" not in orphan_allowlist)
    orphan_group = sorted(read_groups - seeded_groups - {""})
    for coord in orphan_read:
        print(f"  G1 ORPHAN_READ: {coord} 代码里有读取点但种子没登记"
              "——每次读都 miss、每次都回落 fallback，运维在页面上看不到这个旋钮")
    for g in orphan_group:
        print(f"  G2 ORPHAN_GROUP: {g} 代码里出现了种子中没有的组名"
              "——整组都接错了，或组名改了种子忘了跟")

    # 另一端的对账：前端在读、但下发面白名单没给这个键 ⇒ 每次都 miss、每次都回落兜底常量，
    # 运维在页面上改了也不生效（和 G1 ORPHAN_READ 同一形状，只是发生在前端）。
    for coord in sorted(frontend_reads):
        g, _ = coord.split(".", 1)
        if coord in frontend_cross:
            continue
        if g not in frontend_groups or coord in frontend_excluded:
            read_not_dispatched.append(coord)

    # 跨组放行清单自身的两条自检：写错一个字母的坐标会一路静默到前端回落兜底值，
    # 而页面上那条参数看着完全正常（组在白名单里的键不会有这种形态，因为组名是解析出来的）。
    seeded_coords = {f"{g}.{k}" for g, k in seeded}
    for coord in sorted(frontend_cross):
        if coord not in seeded_coords:
            fail(f"runtimeCrossGroupKeys 里的 {coord} 在种子里查无此键"
                 "——下发面会永远返回空值，前端静默回落兜底，运维改了不起作用", 1)
        if coord in frontend_excluded:
            fail(f"runtimeCrossGroupKeys 与 runtimePublicKeys 同时列出了 {coord}"
                 "——一条既放行又排除，实现里先排除后放行的顺序会变成第二个读取面的隐式契约", 1)

    for coord in read_not_dispatched:
        print(f"  FRONTEND_READ_NOT_DISPATCHED: {coord} 前端有 rt* 读取点，但 "
              "runtimePublicGroups/runtimePublicKeys 不会把它下发——前端永远拿不到库里的值，"
              "运维改了也不生效；要么把组/键放进下发面，要么删掉那个读取点")

    if (undeclared or stale or orphan_read or orphan_group or retired_but_wired
            or reason_missing or read_not_dispatched):
        print("❌ 参数中心读取点门未通过")
        return 1
    print("✅ 参数中心读取点门通过：每条要么有人读，要么自己承认没接线，要么已下架并写清了"
          "下架理由；且没有孤儿读取点（读了种子没登记的键）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
