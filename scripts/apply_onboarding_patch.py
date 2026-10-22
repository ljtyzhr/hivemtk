#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把「页面导览文案修订」合并回批次 JSON。

用途：浏览器里点过之后发现锚点换了 / 文案跟锚点对不上，就要改文案。
改文案时 9 个语种各写一遍很容易漏语种或写串行，所以把修订集中放在一个
JSON 里，形状是：

    {
      "<pageKey>": {
        "summary":       { "<lang>": "…" },            # 可选，整页定位说明
        "<stepKey>": {
          "title":       { "<lang>": "…" },            # 该步标题
          "description": { "<lang>": "…" }             # 该步说明
        }
      }
    }

由本脚本按语种合并回对应批次文件；缺语种直接报错（9 个语言必须齐，
少一个 coverage 门就会红）。

用法：
    python3 scripts/apply_onboarding_patch.py <patch.json> <批次 JSON> [...]
"""
import json
import sys

LANGS = ["zh", "en", "ja", "ar", "es", "fr", "de", "ru", "pt"]


def main():
    if len(sys.argv) < 3:
        raise SystemExit(__doc__)
    with open(sys.argv[1], encoding="utf-8") as fh:
        patch = json.load(fh)
    batch_files = sys.argv[2:]

    batches = []
    for p in batch_files:
        with open(p, encoding="utf-8") as fh:
            batches.append((p, json.load(fh)))

    for page_key, sections in patch.items():
        for section_key, fields in sections.items():
            # summary 段直接就是「{lang: 文案}」；其余段是「{字段: {lang: 文案}}」
            if section_key == "summary" and set(fields.keys()) & set(LANGS):
                for lang in LANGS:
                    if lang not in fields:
                        raise SystemExit(f"修订缺语种：{page_key}.summary.{lang}")
                    value = fields[lang]
                    hit = False
                    for _path, data in batches:
                        page = data.get(lang, {}).get(page_key)
                        if not page:
                            continue
                        page["summary"] = value.get("summary", "") if isinstance(value, dict) else value
                        hit = True
                    if not hit:
                        raise SystemExit(f"批次里找不到页面 {page_key}（语种 {lang}）")
                continue
            for field_name, per_lang in fields.items():
                for lang in LANGS:
                    if lang not in per_lang:
                        raise SystemExit(f"修订缺语种：{page_key}.{section_key}.{field_name}.{lang}")
                    value = per_lang[lang]
                    hit = False
                    for _path, data in batches:
                        page = data.get(lang, {}).get(page_key)
                        if not page:
                            continue
                        step = page.setdefault(section_key, {})
                        if field_name not in ("title", "description"):
                            raise SystemExit(f"未知字段 {field_name}")
                        step[field_name] = value
                        hit = True
                    if not hit:
                        raise SystemExit(f"批次里找不到页面 {page_key}（语种 {lang}）")

    for path, data in batches:
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(data, fh, ensure_ascii=False, indent=2)
        print(f"已更新 {path}")


if __name__ == "__main__":
    main()
