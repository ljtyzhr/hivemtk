#!/usr/bin/env python3
"""G5：参数 key 的命名规范 + 全局唯一 + group 收敛。

为什么需要它：读点门（G1/G2）管的是"有没有人读"，defaults 门（G3）管的是
"默认值对不对"，可调旋钮门（G4）管的是"新旋钮有没有入库"。三道门都放行的
情况下，这两道**结构性**约束没人守：

  1) **命名**。key 是运维在页面上看到的字，也是 `GET /api/config/runtime`
     下发给前端的 JSON 字段名。一旦上线就是公开契约，`Channel.Tg.MaxLen`
     改成 `channel_tg_max_len` 之后所有已改过值的站点都会静默回落到默认值
     ——旧 key 读 miss，新 key 没值。改 key 不是重构，是数据迁移。
     所以规范必须在**新增时**就卡住。
  2) **唯一**。库里的唯一约束是 `(param_group, key)` 复合唯一（v3.47.0 迁移
     之后）。同一组内重名是真的坏数据：读取侧按两列定位，重名会更新到错误的
     行。所以这条不靠数据库报错兜底——种子是启动期插入的，撞了之后走的是
     `SeedConfigParams` 的 Errorf 日志，而不是进程退出。

口径（rc=1）：
  - `BAD_KEY`  key 不匹配 `^[a-z][a-z0-9_]*$`、长度 > 64、或与组名无关地
    重复了同一个词的分割写法（如 `foo_foo_bar`）。
  - `GROUP_KEY_REPEAT`  key 里出现连续重复的词片段（`dedup_dedup_ttl`），
    基本是复制粘贴留下的。
  - `RESERVED_PREFIX`  key 以 `_` / 数字开头，或与 SQL 关键字同名。
  - `GROUP_NOT_LOWER` / `GROUP_TOO_LONG`  group 名同样受规范约束。
  - `DUPLICATE`  同一条种子出现两次（`DefaultParamDefs()` 里字面量重复）。

命名规约为什么定成 snake_case 小写：
  - 前端 runtime 端点把 key 原样当 JSON 字段下发，小写 key 在 JS 里访问
    不用写 `obj["camelCase"]`。
  - URL 与 REST 路径（`PUT /api/manage/config-params/:group/:key`）里小写
    不需要转义。
  - 现有 200+ 条种子已经全是小写 snake，规则与存量一致，不制造一次性改名。

`--fix-hint` 会把每条违规附上建议的规范写法，方便直接照着改。
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
RUNTIME_REL = Path("user-server") / "internal" / "controller" / "config_param_runtime.go"

KEY_RE = re.compile(r"^[a-z][a-z0-9_]*$")
GROUP_RE = re.compile(r"^[a-z][a-z0-9_]*$")
MAX_KEY_LEN = 64
MAX_GROUP_LEN = 32

# 与 check-config-param-readpoints.py 同源的条目解析正则。必须逐组 finditer
# 取五元组，不能用 findall —— findall 返回的是分组元组，计数会全错。
ENTRY = re.compile(
    r'\{\s*Group:\s*"([^"]+)"\s*,\s*Key:\s*"([^"]+)"\s*,\s*Name:\s*"([^"]*)"\s*,\s*'
    r'Description:\s*"([^"]*)"\s*,\s*ValueType:\s*"([^"]*)"',
    re.S)

SQL_RESERVED = {
    "order", "group", "select", "table", "index", "key", "value", "user",
    "default", "all", "and", "not", "null", "limit", "offset", "from",
    "where", "join", "left", "right", "inner", "outer", "union", "case",
}


def fail(msg: str, code: int = 2):
    print(f"❌ {msg}")
    sys.exit(code)


def suggest(s: str) -> str:
    """把任意字面量折成规范的小写 snake_case，作为提示用。"""
    s = s.replace("-", "_").replace(".", "_").replace(" ", "_")
    s = re.sub(r"(.)([A-Z][a-z]+)", r"\1_\2", s)
    s = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", s)
    s = re.sub(r"[^A-Za-z0-9]+", "_", s)
    s = re.sub(r"_+", "_", s).strip("_").lower()
    s = re.sub(r"^_+", "", s)
    if not s:
        return "<无法自动推导，请手工命名>"
    if not s[0].isalpha():
        s = "k_" + s
    return s[:MAX_KEY_LEN].rstrip("_")


def main() -> int:
    seeds = REPO_ROOT / SEEDS_REL
    if not seeds.is_file():
        fail(f"种子文件不存在: {seeds}")
    text = seeds.read_text(encoding="utf-8")

    entries = [(m.group(1), m.group(2), m.group(3), m.group(4), m.group(5))
               for m in ENTRY.finditer(text)]
    if not entries:
        fail("一条种子都没解析到：ENTRY 正则与种子格式不符（或种子被挪了文件）")

    bad: list[str] = []
    seen: dict[tuple[str, str], int] = {}
    repeat: list[str] = []

    for group, key, _name, _desc, vtype in entries:
        coord = f"{group}.{key}"

        if not GROUP_RE.match(group):
            bad.append(f"BAD_GROUP: {coord}  group 应为小写 snake_case（{suggest(group)}）")
        elif len(group) > MAX_GROUP_LEN:
            bad.append(f"GROUP_TOO_LONG: {coord}  group 长 {len(group)} > {MAX_GROUP_LEN}")

        if not KEY_RE.match(key):
            bad.append(f"BAD_KEY: {coord}  key 应为小写 snake_case（{suggest(key)}）")
        elif len(key) > MAX_KEY_LEN:
            bad.append(f"KEY_TOO_LONG: {coord}  key 长 {len(key)} > {MAX_KEY_LEN}")
        if key.lower() in SQL_RESERVED:
            bad.append(f"RESERVED_KEY: {coord}  key 与 SQL 关键字同名，拼进查询易漏转义")

        # 连续重复片段：dedup_dedup_ttl / max_max_size 这类复制粘贴残留
        toks = key.split("_")
        for a, b in zip(toks, toks[1:]):
            if a == b and len(a) > 1:
                repeat.append(f"GROUP_KEY_REPEAT: {coord}  key 里 “{a}” 连续出现两次")
                break

        seen[(group, key)] = seen.get((group, key), 0) + 1

    dupes = [f"DUPLICATE: {g}.{k} 在 DefaultParamDefs() 里出现了 {n} 次"
             for (g, k), n in seen.items() if n > 1]

    # group 白名单（只做提示，不卡红）：运行时下发端点里的组必须在种子里真实存在，
    # 否则 runtime 会静默空下发，前端拿到 undefined 回落本地常量——
    # 又是"配了不生效"的一类。已在 G1 的 ORPHAN_GROUP 覆盖，这里做交叉提醒。
    runtime = REPO_ROOT / RUNTIME_REL
    runtime_groups: list[str] = []
    if runtime.is_file():
        rt = runtime.read_text(encoding="utf-8")
        # 两种类型都要认：runtimePublicGroups 是 map[string]struct{}，
        # runtimePublicKeys 是 map[string]bool。写死一种类型的话，
        # 另一处会解析出 0 项——而"扫到 0 个组"看上去和"没有违规"一模一样。
        m = re.search(
            r"runtimePublicGroups\s*=\s*map\[string\](?:struct\{\}|bool)\{(.*?)\n\}",
            rt, re.S)
        if m:
            runtime_groups = re.findall(r'"([a-z0-9_]+)"\s*:', m.group(1))
        else:
            fail(f"{RUNTIME_REL} 里解析不到 runtimePublicGroups："
                 "端点白名单改了形状，本门的交叉检查会静默变成空检查")
    seed_groups = {g for g, _k, _n, _d, _v in entries}
    ghost = [g for g in runtime_groups if g not in seed_groups]

    print(f"扫描面: 种子条目 {len(entries)} 条 / group {len(seed_groups)} 个 / "
          f"runtime 白名单组 {len(runtime_groups)} 个")
    print(f"读数: 命名违规={len(bad)}  重复词={len(repeat)}  重复条目={len(dupes)}  "
          f"runtime 幽灵组={len(ghost)}")
    for line in bad + repeat + dupes:
        print(f"  {line}")
    for g in ghost:
        print(f"  GHOST_GROUP: runtime 白名单里的 {g!r} 在种子里不存在，会静默空下发")

    if bad or repeat or dupes or ghost:
        print("❌ 键规范门未通过：key 一旦上线就是公开契约，改名等于让已配站点静默回落")
        return 1
    print("✅ 键规范门通过：key/group 命名规范、组内唯一、runtime 白名单无幽灵组")
    return 0


if __name__ == "__main__":
    sys.exit(main())