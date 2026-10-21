#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""gen_router_list.py — 由 user-web/src/router/* 生成 router-list.md 索引（任务四 第一步）。

只做「索引生成」：把前端页面路由逐条列成清单，供后续浏览器逐页点击验证使用。
本脚本**不开浏览器**、不发请求、不改代码。

抽取口径：
  - src/router/index.js 里的 initRoutes（固定路由：setup / login / 帮助中心 / 聊天挂件）
  - src/router/modules/*.js 里的模块路由（业务页面）
  - 记录 path / name / 组件文件 / 是否需登录 / 标题
"""
import os
import re
from collections import OrderedDict

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ROUTER_DIR = os.path.join(ROOT, "user-web", "src", "router")
MOD_DIR = os.path.join(ROUTER_DIR, "modules")
OUT = os.path.join(ROOT, "router-list.md")

# 路由对象字段之间常夹注释行（`// ...`），先剥掉再匹配，否则 path/name 之间的
# 注释会把正则打断、整条路由静默漏掉。
COMMENT_RE = re.compile(r"^\s*//.*$", re.M)

# 一个路由对象：{ path: '...', name: '...', component: () => import('@/views/X.vue'), meta: {...} }
ROUTE_RE = re.compile(
    r"\{\s*path:\s*'([^']*)'\s*,\s*name:\s*'([^']*)'\s*,\s*"
    r"component:\s*\(\)\s*=>\s*import\('([^']*)'\)"
    r"(?:\s*,\s*meta:\s*\{([^}]*)\})?",
    re.S,
)
# 重定向路由：{ path: '...', name: '...', redirect: '...', meta: {...} }
# 与页面路由一样可能跨行，且 name/meta 顺序不固定。
REDIRECT_RE = re.compile(
    r"path:\s*'([^']*)'\s*,\s*name:\s*'([^']*)'\s*,\s*redirect:\s*'([^']*)'"
    r"(?:\s*,\s*meta:\s*\{([^}]*)\})?",
    re.S,
)
# 具名 layout 根路由：{ path: '/', name: 'Layout', component: Layout, redirect: '...' }
LAYOUT_RE = re.compile(
    r"path:\s*'([^']*)'\s*,\s*name:\s*'([^']*)'\s*,\s*component:\s*Layout"
    r"(?:\s*,\s*redirect:\s*'([^']*)')?",
    re.S,
)
# 匿名重定向：{ path: '...', redirect: '...' }（无 name）
ANON_REDIRECT_RE = re.compile(
    r"path:\s*'([^']*)'\s*,\s*redirect:\s*'([^']*)'", re.S
)
TITLE_RE = re.compile(r"title:\s*'([^']*)'")
AUTH_RE = re.compile(r"requiresAuth:\s*(true|false)")


def parse_routes(src):
    out = []
    seen = set()
    # 剥注释：字段之间夹的 `// ...` 会打断 path/name 的相邻匹配
    clean = COMMENT_RE.sub("", src)
    for m in ROUTE_RE.finditer(clean):
        path, name, comp, meta = m.groups()
        title = ""
        auth = ""
        if meta:
            t = TITLE_RE.search(meta)
            a = AUTH_RE.search(meta)
            title = t.group(1) if t else ""
            auth = a.group(1) if a else ""
        seen.add(path)
        out.append(
            OrderedDict(
                path=path, name=name, comp=comp.replace("@/", ""),
                title=title, auth=auth, kind="页面",
            )
        )
    for m in REDIRECT_RE.finditer(clean):
        path, name, target, meta = m.groups()
        if path in seen:
            continue
        seen.add(path)
        title = ""
        auth = ""
        if meta:
            t = TITLE_RE.search(meta)
            a = AUTH_RE.search(meta)
            title = t.group(1) if t else ""
            auth = a.group(1) if a else ""
        out.append(
            OrderedDict(
                path=path, name=name, comp="-", title=title, auth=auth,
                kind="重定向→" + target,
            )
        )
    for m in LAYOUT_RE.finditer(clean):
        path, name, target = m.groups()
        if path in seen:
            continue
        seen.add(path)
        out.append(
            OrderedDict(
                path=path, name=name, comp="layout/Layout.vue", title="", auth="",
                kind="重定向→" + (target or "-"),
            )
        )
    for m in ANON_REDIRECT_RE.finditer(clean):
        path, target = m.groups()
        if path in seen:
            continue
        seen.add(path)
        out.append(
            OrderedDict(
                path=path, name="-", comp="-", title="", auth="",
                kind="重定向→" + target,
            )
        )
    return out


def main():
    rows = []

    # 固定路由
    with open(os.path.join(ROUTER_DIR, "index.js"), encoding="utf-8") as fh:
        idx = fh.read()
    for r in parse_routes(idx):
        r["src"] = "src/router/index.js"
        rows.append(r)

    # 模块路由
    for fname in sorted(os.listdir(MOD_DIR)):
        if not fname.endswith(".js"):
            continue
        with open(os.path.join(MOD_DIR, fname), encoding="utf-8") as fh:
            src = fh.read()
        for r in parse_routes(src):
            r["src"] = f"src/router/modules/{fname}"
            rows.append(r)

    lines = []
    lines.append("# 前端页面路由清单（任务四 · SaaS 多租户改造后逐页面拟人点击验证）")
    lines.append("")
    lines.append(f"- 来源：`user-web/src/router/index.js` + `src/router/modules/*.js`")
    lines.append(f"- 规模：**{len(rows)} 条页面路由**")
    lines.append("- 生成方式：`scripts/gen_router_list.py`（只做索引，不开浏览器）")
    lines.append("- 校验口径：每条路由**逐页打开**，点击页面所有元素/表单/交互，")
    lines.append("  比对浏览器控制台报错与 Network 里的 API 响应；发现错误即修复")
    lines.append("")
    lines.append("## 清单")
    lines.append("")
    lines.append("| # | 状态 | 路径 | 名称 | 标题 | 需登录 | 组件 | 源文件 |")
    lines.append("|---|---|---|---|---|---|---|---|")
    for i, r in enumerate(rows, 1):
        auth = "是" if r["auth"] == "true" else ("否" if r["auth"] == "false" else "-")
        lines.append(
            f"| {i} | [ ] | `{r['path']}` | {r['name']} | {r['title']} | {auth} | "
            f"`{r['comp']}` | `{r['src']}` |"
        )
    lines.append("")

    with open(OUT, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))
    print(f"写入 {OUT}：{len(rows)} 条页面路由")


if __name__ == "__main__":
    main()
