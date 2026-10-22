#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把「页面导览」文案块插进 9 个语言包的 setup.onboarding.page 下。

只做**文本级拼接**（不是 JSON 往返），理由有两个：
  1. 语言包里存在**重复键**（如「平台」「系统」在 zh.json 里各出现两次），
     JSON 往返会把先出现的那个丢掉——实测一进一出就少 37 个字符的词条。
  2. 文本级拼接能原样保留各语言包**已有的排版差异**（zh/en 的 step 是单行
     对象，ja/ar/es/… 是展开成多行的），不制造无谓的 diff。

用法：
    python3 scripts/i18n_add_onboarding_pages.py <批次 JSON> [...]
    批次 JSON 结构：{ "<lang>": { "<pageKey>": { "summary": ..., "step2": {"title","description"} } } }
    （缺哪个语言就跳过那个语言，但 9 个语言必须齐——缺了 i18n 覆盖门会红）
"""
import json
import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LOCALES_DIR = os.path.join(ROOT, "user-web", "src", "i18n", "locales")
LANGS = ["zh", "en", "ja", "ar", "es", "fr", "de", "ru", "pt"]


def esc(s):
    return json.dumps(s, ensure_ascii=False)


def find_block(lines, key_line_startswith, indent):
    """在文本里定位某个对象的起止行（按花括号配平），返回 (start, end) 行号（0 基、含端点）。"""
    start = None
    for i, l in enumerate(lines):
        if l.strip().startswith(key_line_startswith) and l.startswith(indent):
            start = i
            break
    if start is None:
        raise SystemExit(f"找不到 {key_line_startswith!r} 起始行")
    depth = 0
    for i in range(start, len(lines)):
        depth += lines[i].count("{") - lines[i].count("}")
        if depth == 0:
            return start, i
    raise SystemExit(f"{key_line_startswith!r} 的花括号没配平")


def build_page_block(page_key, data, indent):
    """生成一个页面在 setup.onboarding.page 下的文本块。"""
    pad = indent
    L = []
    L.append(f'{pad}"{page_key}": {{')
    L.append(f'{pad}  "summary": {esc(data["summary"])},')
    step_keys = [k for k in data if k != "summary"]
    for i, step_key in enumerate(step_keys):
        step = data[step_key]
        L.append(f'{pad}  "{step_key}": {{')
        L.append(f'{pad}    "title": {esc(step["title"])},')
        L.append(f'{pad}    "description": {esc(step["description"])}')
        L.append(f'{pad}  }}' + ("," if i < len(step_keys) - 1 else ""))
    L.append(f'{pad}}}')
    return L


def main():
    if len(sys.argv) < 2:
        raise SystemExit(__doc__)
    batches = []
    for path in sys.argv[1:]:
        with open(path, encoding="utf-8") as fh:
            batches.append(json.load(fh))
    # 合并多批次
    merged = {lang: {} for lang in LANGS}
    for b in batches:
        for lang, pages in b.items():
            if lang not in merged:
                raise SystemExit(f"未知语言 {lang}")
            merged[lang].update(pages)

    for lang in LANGS:
        pages = merged[lang]
        if not pages:
            continue
        p = os.path.join(LOCALES_DIR, f"{lang}.json")
        with open(p, encoding="utf-8") as fh:
            lines = fh.read().split("\n")
        # 定位 setup.onboarding.page 的对象（含其大括号）
        start, end = find_block(lines, '"page": {', "      ")
        # 找到 page 对象的收尾大括号行
        insert_at = end
        while not lines[insert_at].strip().startswith("}"):
            insert_at -= 1
        # page 对象里已存在的最后一个键的行，需要补一个逗号（它后面要接新块）
        prev = insert_at - 1
        while not lines[prev].strip():
            prev -= 1
        if not lines[prev].rstrip().endswith(","):
            lines[prev] = lines[prev].rstrip() + ","
        block = []
        for i, (page_key, data) in enumerate(pages.items()):
            blk = build_page_block(page_key, data, "        ")
            if i < len(pages) - 1:
                blk[-1] = blk[-1] + ","
            block.extend(blk)
        new_lines = lines[:insert_at] + block + lines[insert_at:]
        with open(p, "w", encoding="utf-8") as fh:
            fh.write("\n".join(new_lines))
        # 校验 JSON 合法
        with open(p, encoding="utf-8") as fh:
            data = json.load(fh)
        got = list(data["setup"]["onboarding"]["page"].keys())
        missing = [k for k in pages if k not in got]
        if missing:
            raise SystemExit(f"{lang}: 插入后仍缺 {missing}")
        print(f"{lang}: 写入 {len(pages)} 个页面导览 -> {list(pages.keys())}")


if __name__ == "__main__":
    main()
