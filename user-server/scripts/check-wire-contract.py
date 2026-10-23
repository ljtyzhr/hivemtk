#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""跨仓线上契约比对门禁（SaaS 审计 A10）。

# 它在挡什么

平台端与 user-server 是两个仓、两套 Go 类型，描述同一批"跨端传递的结构"（开户登记 /
心跳 / 订单 / 模块下发）。Go 编译器看不见另一侧，于是**改一边的 json tag、两边都能编译、
两边用例都能绿**，线上的表现是：对面的键被静默忽略，字段永远是零值。这类 bug 不报错、
不进日志，只在几个月后有人问"为什么这个开关改了没反应"时才被翻出来。

本脚本把两份**从活体 struct 反射出来**的清单放在一起比：

    platform-server/docs/contracts/cross-repo-wire.json   ← 权威方（服务端定义接受什么）
    user-server/docs/contracts/cross-repo-wire.json       ← 消费方

# 三条判据

① **类型集相等**：两侧清单里的 wire 类型必须一一对上（键相等）。两侧的 Go 名字本来就
   不同（`ModuleCatalogRow` ↔ `MerchantModuleView`），但清单是生成物，租户侧生成时就
   把它改写成平台侧的名字 —— 于是"谁对谁"退化成键相等，**不存在一张会漏行的映射表**。
② **字段方向性子集**：租户发/读的每个键，平台侧必须有。平台合法地比租户多
   （`owned` / `menu_keys` / `asset_ids` / `merchant_key` 只服务平台自身），所以是子集
   而不是相等 —— 相等会造出永久红，而永久红的判据等于没有判据。
③ **必填反向**：平台 `binding:"required"` 的键，租户必须发。漏了是**整条通道 400**，
   而且只在"租户端刚好升级到会发这个键的那一版 / 平台先升上去"那一刻才炸。

# 为什么比对 JSON 形状而不是 Go 类型

两端合法地会用不同的 Go 类型表达同一个线上值：`ModuleSyncSnapshot.SyncedAt` 平台侧是
`time.Time`（线上 RFC3339 字符串），租户侧是 `string`。按 Go 类型比会永远红。所以清单
只记 wire 层口径（string/integer/number/boolean/array/object/any）+ 是否可空。

退出码：0 通过；1 有漂移；2 环境不成立（找不到清单/对面仓），**也是失败** ——
"没跑起来"与"跑过了没问题"必须能分开。
"""

import argparse
import json
import os
import sys

# 兼容口径：integer 与 number 在线上都能被对方解析（int64 ↔ float64）。
COMPATIBLE = {("integer", "number"), ("number", "integer")}

# 四条通道的根类型（A10 的范围）。少一条 ⇒ 该通道退回"靠人对齐"。
ROOTS = [
    "RegisterReq",          # 开户登记-注册
    "InstallReportRequest", # 开户登记-安装
    "HeartbeatRequest",     # 心跳
    "CreateOrderInput",     # 订单-请求
    "CreateOrderResult",    # 订单-响应
    "ModuleSyncSnapshot",   # 模块下发
]

# 允许只在一侧出现的类型：{类型名: 理由}。
#
# 默认空 —— "某个跨端结构只在一侧登记"就该红，因为那意味着另一侧的那条子树不在判据里。
# 真要放行必须写在这里并给出理由，于是"放行"这件事本身是**可审的**：翻这个文件就知道
# 判据被开了哪些口子。不要用删登记项来消红。
ONE_SIDED = {}


def die(msg, code=1):
    print("❌ %s" % msg)
    sys.exit(code)


def load(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            m = json.load(f)
    except FileNotFoundError:
        die("找不到清单 %s —— 它必须进 git（由各仓的 wirecontract 测试生成）" % path, 2)
    except json.JSONDecodeError as e:
        die("清单 %s 不是合法 JSON：%s" % (path, e), 2)
    if "structs" not in m:
        die("清单 %s 缺少 structs 段" % path, 2)
    for name, st in m["structs"].items():
        if "fields" not in st:
            die("清单 %s 里 %s 缺少 fields 段" % (path, name), 2)
        for f in st["fields"]:
            if "json" not in f or "kind" not in f:
                die("清单 %s 里 %s 的某个字段缺少 json/kind" % (path, name), 2)
    return m


def compat_kind(a, b):
    if a == b or (a, b) in COMPATIBLE:
        return True
    # any = json.RawMessage / interface{}：线上形状由调用方决定，不参与比较。
    return a == "any" or b == "any"


def fields_by_json(st):
    return {f["json"]: f for f in st["fields"]}


def compare_struct(tname, tenant, platform, visited, problems, path):
    """比一个类型：方向性子集 + 必填反向，并递归进嵌套类型（键相等即配对）。"""
    if tname in visited:
        return
    visited.add(tname)

    if tname not in tenant["structs"]:
        problems.append("%s：租户侧清单里没有 %s（该类型没有登记进反射清单？）" % (path or tname, tname))
        return
    if tname not in platform["structs"]:
        problems.append("%s：平台侧清单里没有 %s（平台侧删了这个类型，或租户侧的新类型没登记规范名）"
                        % (path or tname, tname))
        return

    tf = fields_by_json(tenant["structs"][tname])
    pf = fields_by_json(platform["structs"][tname])

    # ① 租户发/读的每个键，平台侧必须有；线上口径必须相容
    for name, f in sorted(tf.items()):
        here = "%s.%s" % (path, name) if path else "%s.%s" % (tname, name)
        if name not in pf:
            problems.append(
                "%s：租户侧有这个键，平台侧 %s 没有 —— 线上会被**静默忽略**"
                "（请求方向表现为功能不生效，响应方向表现为永远读到零值）" % (here, tname))
            continue
        g = pf[name]
        if not compat_kind(f["kind"], g["kind"]):
            problems.append(
                "%s：线上口径不一致（租户 %s / 平台 %s）—— 一方序列化出来的东西另一方解不出来"
                % (here, f["kind"], g["kind"]))
            continue
        # 嵌套类型继续往下（array 的元素 / object 的具名类型）
        if f["kind"] in ("array", "object") and f.get("elem"):
            if not g.get("elem"):
                problems.append(
                    "%s：租户侧是复合类型（elem=%s），平台侧没有具名元素类型 —— "
                    "两边说的不是同一个形状" % (here, f["elem"]))
                continue
            if f["elem"] != g["elem"]:
                problems.append(
                    "%s：嵌套类型名对不上（租户 %s / 平台 %s）—— 清单规范名没对齐，"
                    "这条子树因此不在判据里" % (here, f["elem"], g["elem"]))
                continue
            compare_struct(f["elem"], tenant, platform, visited, problems,
                           "%s.%s" % (path, name) if path else "%s.%s" % (tname, name))

    # ② 平台 required 的键，租户必须发
    for name, g in sorted(pf.items()):
        if g.get("required") and name not in tf:
            problems.append(
                "%s：平台侧把 %s 标成 required（binding:\"required\"），租户侧从来不发 —— "
                "线上是**整条通道 400**，而且只在租户升级那一刻才炸" % (tname, name))


def check_manifest_version(tenant, platform):
    """两份清单的**口径版本**必须相同。

    两个仓各有一份 `wirecontract` 反射器（不共代码，见 README）。若一边把
    `time.Time` 算成 string、另一边算成 integer，那两份清单的字段口径就悄悄分叉了 ——
    而这种分叉不会以"某个字段不一致"的形式出现，它会以一堆莫名其妙的一致性告警出现。
    口径版本号相等是这份分叉存在与否的最低成本判据。
    """
    tv, pv = tenant.get("version"), platform.get("version")
    if tv != pv:
        return ["两份清单的口径版本不一致（租户 %r / 平台 %r）—— 两仓的 wirecontract 反射器"
                "口径分叉了，必须两仓同批改" % (tv, pv)]
    return []


def check_type_sets(tenant, platform):
    problems = []
    for name in ROOTS:
        for side, m in (("租户", tenant), ("平台", platform)):
            if name not in m["structs"]:
                problems.append("%s侧清单缺根类型 %s —— 四条通道的这条会退回「靠人对齐」"
                                % (side, name))
    t, p = set(tenant["structs"]), set(platform["structs"])
    for name in sorted(t - p - set(ONE_SIDED)):
        problems.append("类型 %s 只在租户侧清单里 —— 平台侧没有对应结构（改错了名字？）"
                        "；若确实只有一侧使用，请登记进 ONE_SIDED" % name)
    for name in sorted(p - t - set(ONE_SIDED)):
        problems.append("类型 %s 只在平台侧清单里 —— 租户侧没有对应结构"
                        "；若确实只有一侧使用，请登记进 ONE_SIDED" % name)
    return problems


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--tenant-manifest", default="docs/contracts/cross-repo-wire.json")
    ap.add_argument("--platform-manifest", default=None,
                    help="默认从兄弟仓 ../hivemtk-platform/platform-server/docs/contracts/ 找")
    ap.add_argument("--allow-missing-platform", action="store_true",
                    help="单仓 CI（只有本仓 checkout）时跳过跨仓比对；**会响亮打印**，不静默")
    args = ap.parse_args()

    here = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    tenant_path = args.tenant_manifest
    if not os.path.isabs(tenant_path):
        tenant_path = os.path.join(here, tenant_path)

    platform_path = args.platform_manifest
    if platform_path is None:
        # 兄弟仓常见落法：与本仓同级（../hivemtk-platform）或与仓根同级
        # （../../hivemtk-platform）。两个都试，第一个存在的为准 —— 写死一种落法会让
        # "换一台机器就跑不起来"，而跑不起来时的退出码必须与"跑过了有漂移"分开。
        rel = os.path.join("hivemtk-platform", "platform-server", "docs", "contracts",
                           "cross-repo-wire.json")
        candidates = [os.path.join(os.path.dirname(here), rel),
                      os.path.join(os.path.dirname(os.path.dirname(here)), rel)]
        platform_path = next((p for p in candidates if os.path.exists(p)), candidates[0])

    print("=" * 78)
    print("  跨仓线上契约比对（类型集相等 + 租户字段 ⊆ 平台字段 + 平台必填 ⊆ 租户）· 审计 A10")
    print("=" * 78)
    print("  租户侧清单: %s" % tenant_path)

    tenant = load(tenant_path)

    if not os.path.exists(platform_path):
        if not args.allow_missing_platform:
            die("找不到平台侧清单 %s。\n"
                "       双仓都在的机器上用 `--platform-manifest` 指过去；\n"
                "       单仓 CI 请显式加 `--allow-missing-platform`（跳过跨仓比对这件事必须被写出来，"
                "不能靠默认值悄悄发生）" % platform_path, 2)
        print("  平台侧清单: **未找到 —— 跨仓比对被显式跳过**（%s）" % platform_path)
        print("  ⚠️  单仓 CI 的覆盖到此为止：本仓清单只能证明「struct 与清单一致」，")
        print("      证明不了「与平台侧一致」。发布前请在双仓都在的机器上跑一次。")
        # 仍然做本仓的自洽检查（四条通道的根类型都在），避免"跳过"变成"什么都不查"。
        problems = [p for p in check_type_sets(tenant, tenant) if "平台" not in p]
        if problems:
            for p in problems:
                print("  ❌ %s" % p)
            sys.exit(1)
        print("  ✅ 本仓清单自洽（四条通道根类型齐全）；跨仓比对未执行")
        return 0

    print("  平台侧清单: %s" % platform_path)
    platform = load(platform_path)

    problems = check_manifest_version(tenant, platform) + check_type_sets(tenant, platform)
    visited = set()
    for root in ROOTS:
        if root in tenant["structs"] and root in platform["structs"]:
            compare_struct(root, tenant, platform, visited, problems, "")

    # 类型集相等之后，凡是"清单里有但没被走到"的类型说明它没有挂进四条通道的任何一条
    # —— 那是"登记了但没人比"的静默空转，也要说出来。
    for name in sorted(set(tenant["structs"]) & set(platform["structs"]) - visited):
        if name not in ONE_SIDED:
            print("  · 提示：类型 %s 在两侧清单里都有，但没有从任何根类型可达（跳过比对）" % name)

    print()
    if problems:
        for p in problems:
            print("  ❌ %s" % p)
        print()
        print("❌ 跨端线上契约漂移 %d 处（SaaS 审计 A10）" % len(problems))
        return 1

    print("  · 根类型 %d 个；两侧类型集一致 %d 个；实际比对（含嵌套）%d 个"
          % (len(ROOTS), len(tenant["structs"]), len(visited)))
    print("  ✅ 无漂移：租户侧发/读的每个键平台侧都认识，平台的必填键租户侧都发")
    return 0


if __name__ == "__main__":
    sys.exit(main())
