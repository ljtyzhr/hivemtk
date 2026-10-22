#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把「通知中心」页的中文字面量键收进 notifications.* 命名空间。

背景：i18n 棘轮门把「中文字面量直传」（raw_t_calls）当劣化项。通知中心页在
2026-10-21 那批提交里新增了 6 条中文字面量键（级别 / 平台下发 / 平台消息 /
普通 / 重要 / 系统通知），把 raw_t_calls 从基线 1167 顶到 1173，门在 HEAD 上
就已经是红的。

做法：
  1. 9 个语言包各加一个 notifications 命名空间（沿用已有的译文，不重译）；
  2. 页面里改成点分键；
  3. 删掉因此失去消费方的 6 条中文字面量键。

净效果：raw_t_calls 回到基线以内，且不需要动基线文件。
"""
import json
import os
import re

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LOCALES_DIR = os.path.join(ROOT, "user-web", "src", "i18n", "locales")
LANGS = ["zh", "en", "ja", "ar", "es", "fr", "de", "ru", "pt"]

# 中文字面量键 -> notifications 命名空间下的键名
SITE = {
    "级别": "level",
    "平台下发": "fromPlatform",
    "平台消息": "platformNotice",
    "普通": "normal",
    "重要": "important",
    "系统通知": "systemNotice",
}

# 页面里改动的调用形态
CALL_FORMS = [
    ("$t('%s')", "$t('notifications.%s')"),
    ("i18n.global.t('%s')", "i18n.global.t('notifications.%s')"),
]


def esc(s):
    return json.dumps(s, ensure_ascii=False)


def main():
    # 1) 收集各语言里这 6 条键的现有译文
    values = {}
    for lang in LANGS:
        p = os.path.join(LOCALES_DIR, f"{lang}.json")
        data = json.load(open(p, encoding="utf-8"))
        values[lang] = {}
        for zh_key, sub in SITE.items():
            if zh_key not in data:
                raise SystemExit(f"{lang}.json 缺词条 {zh_key}")
            values[lang][sub] = data[zh_key]

    # 2) 每个语言包插入 notifications 命名空间 + 删掉失去消费方的字面量键
    for lang in LANGS:
        p = os.path.join(LOCALES_DIR, f"{lang}.json")
        lines = open(p, encoding="utf-8").read().split("\n")

        if '"notifications": {' not in "\n".join(lines):
            si = next(i for i, l in enumerate(lines) if l.strip().startswith('"setup": {'))
            block = ['  "notifications": {']
            items = list(values[lang].items())
            for i, (k, v) in enumerate(items):
                block.append(f'    "{k}": {esc(v)}' + ("," if i < len(items) - 1 else ""))
            block.append("  },")
            lines = lines[:si] + block + lines[si:]

        text = "\n".join(lines)
        for zh_key in SITE:
            text = re.sub(r'\n  "%s":\s*"(?:[^"\\]|\\.)*",?' % re.escape(zh_key), "", text)
        lines = text.split("\n")

        open(p, "w", encoding="utf-8").write("\n".join(lines))

        data = json.load(open(p, encoding="utf-8"))
        assert set(values[lang]) <= set(data["notifications"]), lang
        print(f"{lang}: notifications 命名空间就绪（{len(values[lang])} 条），已清 {len(SITE)} 条字面量键")

    # 3) 页面改成点分键
    vue = os.path.join(ROOT, "user-web", "src", "views", "Notifications.vue")
    src = open(vue, encoding="utf-8").read()
    for zh_key, sub in SITE.items():
        for old, new in CALL_FORMS:
            src = src.replace(old % zh_key, new % sub)
    open(vue, "w", encoding="utf-8").write(src)
    for zh_key, sub in SITE.items():
        assert "notifications.%s" % sub in src, f"页面没改到 notifications.{sub}"
        assert ("$t('%s')" % zh_key) not in src, f"页面还留着字面量键 {zh_key}"
    print("Notifications.vue: 取词方式已改为点分键")


if __name__ == "__main__":
    main()
