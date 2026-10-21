#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""gen_api_list.py — 由 user-web/src/api/* 生成 api-list.md 索引（任务三 第一步）。

只做「索引生成」：把前端 API 出口逐条抽成清单，供后续逐路由 curl 验证使用。
本脚本**不做任何请求**，也不批量改代码。

抽取口径：
  - 扫描 user-web/src/api/*.js 的 `http.<method>(<url>[, ...])` 调用
  - 解析同一文件内的 `const base = '...'` 常量并做替换
  - 模板串 `${...}` 段一律归一为 `{param}` 占位符（保留参数名）
  - 记录调用点行号、导出名、http 方法、URL 形态、参数来源
"""
import os
import re
import sys
from collections import OrderedDict

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
API_DIR = os.path.join(ROOT, "user-web", "src", "api")
OUT = os.path.join(ROOT, "api-list.md")

# http.<method>( "url"  或  `url`  或  ${base}/x  或  base（裸标识符）
CALL_RE = re.compile(
    r"http\.(get|post|put|patch|delete|upload)\s*\(\s*(?:(['\"`])([^'\"`]*)\2|([A-Za-z_$][\w$]*))",
    re.M,
)
BASE_CONST_RE = re.compile(r"^\s*(?:export\s+)?const\s+(\w+)\s*=\s*['\"]([^'\"]+)['\"]", re.M)

# 反引号里的 ${expr} → {expr}（expr 里的 . 换成 _，去掉 encodeURIComponent 包裹）
def norm_template(u):
    def rep(m):
        expr = m.group(1)
        expr = re.sub(r"encodeURIComponent\(([^)]*)\)", r"\1", expr)
        expr = re.sub(r"\bseg\(([^)]*)\)", r"\1", expr)   # seg(id) → id
        expr = expr.replace(".", "_")
        expr = expr.replace(" ", "")
        return "{" + expr + "}"
    return re.sub(r"\$\{([^}]*)\}", rep, u)


def classify_auth(path):
    """按 URL 前缀判档位（前端出口档位，后端实际鉴权以路由装配为准）。"""
    pub = (
        "/api/auth/login", "/api/auth/mfa", "/api/auth/refresh-token",
        "/api/module-market/", "/api/tenant/regulatory-notice",
        "/api/chat/public", "/api/health", "/healthz",
    )
    if any(path.startswith(p) for p in pub):
        return "公开"
    if path.startswith("/api/manage/") or path.startswith("/api/platform/"):
        return "管理端"
    return "用户端"


def main():
    rows = []
    files = sorted(f for f in os.listdir(API_DIR) if f.endswith(".js"))
    for fname in files:
        path = os.path.join(API_DIR, fname)
        with open(path, encoding="utf-8") as fh:
            src = fh.read()
        consts = {m.group(1): m.group(2) for m in BASE_CONST_RE.finditer(src)}
        for m in CALL_RE.finditer(src):
            method = m.group(1).upper()
            raw = m.group(3)
            if raw is None:
                # 裸标识符（http.get(base, params)）→ 取同文件常量
                raw = consts.get(m.group(4), "")
            line = src[: m.start()].count("\n") + 1
            url = raw
            for k, v in consts.items():
                if k in url:
                    url = url.replace("${" + k + "}", v).replace(k, v)
            url = norm_template(url)
            rows.append(
                OrderedDict(
                    file=fname, line=line, method=method, url=url,
                    auth=classify_auth(url),
                )
            )

    # 输出
    by_method = {}
    for r in rows:
        by_method.setdefault(r["method"], []).append(r)

    lines = []
    lines.append("# API 路由清单（任务三 · SaaS 多租户改造后逐路由 curl 验证）")
    lines.append("")
    lines.append(f"- 来源：`user-web/src/api/*.js`（{len(files)} 个文件）")
    lines.append(f"- 规模：**{len(rows)} 条前端 API 出口**")
    lines.append("- 生成方式：`scripts/gen_api_list.py`（只做索引，不发请求）")
    lines.append("- 校验口径：每条路由**逐条手工 curl**，分析 `code` 与 `data`；")
    lines.append("  `code=0` 通过；`code!=0` 判定是调用错误还是业务错误，即时修复")
    lines.append("")
    lines.append("## 档位说明")
    lines.append("")
    lines.append("| 档位 | 含义 |")
    lines.append("|---|---|")
    lines.append("| 公开 | 无需登录（登录、模块市场、公开聊天） |")
    lines.append("| 用户端 | 需登录的普通业务接口 `/api/*` |")
    lines.append("| 管理端 | 管理面接口 `/api/manage/*`、`/api/platform/*` |")
    lines.append("")
    lines.append("## 清单")
    lines.append("")
    lines.append("| # | 状态 | 方法 | 路径 | 档位 | 源文件 | 行 |")
    lines.append("|---|---|---|---|---|---|---|")
    for i, r in enumerate(rows, 1):
        lines.append(
            f"| {i} | [ ] | {r['method']} | `{r['url']}` | {r['auth']} | "
            f"`src/api/{r['file']}` | {r['line']} |"
        )
    lines.append("")

    with open(OUT, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))
    print(f"写入 {OUT}：{len(rows)} 条（{len(files)} 个文件）")
    for m in sorted(by_method):
        print(f"  {m}: {len(by_method[m])}")


if __name__ == "__main__":
    main()
