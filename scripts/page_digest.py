#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把一个 Vue 页面压成「结构摘要」，供人（不是脚本）读懂这一页在干什么。

为什么要摘要而不是直接贴源码：单页动辄 1500~3000 行，150 多个页面全读一遍
是几十兆的上下文。摘要只留「这页有哪些功能入口」——标题、表单字段、表格列、
按钮、调的接口、以及源码里的关键注释。读摘要 + 必要时回看局部，比通读全文件
省一个数量级的上下文，而判断「这一页是做什么的、怎么用」所需的信息都在里面。

注意：摘要**只用于读**，文案是人写的；本脚本不生成任何文案。

用法：
    python3 scripts/page_digest.py user-web/src/views/system/Config.vue [更多文件...]
"""
import re
import sys

LABEL = re.compile(r'(?:label|title|placeholder|description|prop)\s*=\s*"([^"]{1,60})"', re.I)
BTN = re.compile(r">\s*([^<>{}\n]{1,40}?)\s*<", re.S)
COMMENT = re.compile(r"<!--(.*?)-->", re.S)
API_CALL = re.compile(r"\b([a-zA-Z_][\w]{2,})\s*\(")
HTTP = re.compile(r"""http\.(get|post|put|delete|patch)\s*\(\s*[`'"]([^`'"]+)""")
TAG = re.compile(r"<(el-[a-z-]+|input|textarea|select|option|button)\b", re.I)


def digest(path):
    src = open(path, encoding="utf-8").read()
    lines = src.split("\n")
    print("=" * 78)
    print("FILE:", path, f"({len(lines)} lines)")
    print("=" *78)

    head = "\n".join(lines[:40])
    m = re.search(r"<!--(.*?)-->", head, re.S)
    if m:
        print("[head comment]", " ".join(m.group(1).split())[:600])

    print("\n[labels]")
    seen = set()
    for lbl in LABEL.findall(src):
        lbl = lbl.strip()
        if lbl and lbl not in seen and len(lbl) > 1:
            seen.add(lbl)
            print("  -", lbl)

    tags = {}
    for m2 in TAG.finditer(src):
        t = m2.group(1)
        tags[t] = tags.get(t, 0) + 1
    print("\n[components]")
    print("  " + ", ".join(f"{k}x{v}" for k, v in sorted(tags.items(), key=lambda kv: -kv[1])[:22]))

    print("\n[template texts]")
    outs = []
    for m3 in BTN.finditer(src):
        t = " ".join(m3.group(1).split())
        if t and 1 < len(t) < 40 and not t.startswith(("{{", "v-", ":", "@")):
            if t not in outs:
                outs.append(t)
    print("  " + " | ".join(outs[:70]))

    print("\n[api calls]")
    apis = []
    for m4 in HTTP.finditer(src):
        apis.append(f"{m4.group(1).upper()} {m4.group(2)}")
    for m4 in API_CALL.finditer(src):
        name = m4.group(1)
        if name.endswith("Api") or name[0].isupper():
            apis.append(f"fn {name}")
    uniq = []
    for a in apis:
        if a not in uniq:
            uniq.append(a)
    print("  " + "; ".join(uniq[:32]))

    print("\n[chinese comments]")
    for i, l in enumerate(lines):
        s = l.strip()
        if s.startswith("//") and re.search(r"[\u4e00-\u9fff]", s):
            t = " ".join(s[2:].split())
            if len(t) > 12:
                print(f"  L{i+1}: {t[:200]}")
    for m5 in COMMENT.finditer(src):
        t = " ".join(m5.group(1).split())
        if re.search(r"[\u4e00-\u9fff]", t) and len(t) > 14:
            print("  <!--", t[:230], "-->")
    print()


if __name__ == "__main__":
    for p in sys.argv[1:]:
        digest(p)
