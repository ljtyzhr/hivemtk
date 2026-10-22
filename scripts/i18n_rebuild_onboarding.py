#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""重建 9 个语言包里的「新手引导」相关词条（任务六）。

背景：任务六把引导从「6 步全局导览」扩成「全局 + 每页一条」，于是每个语言包
都要动三处：
  1. `setup.onboarding` 整块重写（全局导览文案 + `close` 按钮文案 + 页面导览子树）
  2. `layout.pageTour` / `layout.pageTourUnavailable`（用户菜单里「重看本页引导」）
  3. `customReport.dimension.*` / `customReport.metric.*`（自定义报表的维度/指标标签）

为什么用**文本级拼接**而不是 JSON 往返：
  - 语言包里存在**重复键**（zh.json 里「平台」「系统」各出现两次）。JSON 往返
    只保留最后一个，先出现的那个会连同它的词条一起消失（实测一次往返少 37 个
    字符的词条，coverage 门会跟着红）。
  - 各语言包**排版不同**（zh/en 的 step 是单行对象，其余语言是展开成多行的）。
    文本级拼接保留原有排版，diff 只包含真正改动的部分。

用法：
    python3 scripts/i18n_rebuild_onboarding.py <global.json> [pages.json ...]
"""
import json
import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LOCALES_DIR = os.path.join(ROOT, "user-web", "src", "i18n", "locales")
LANGS = ["zh", "en", "ja", "ar", "es", "fr", "de", "ru", "pt"]

# 通用按钮词条（各语言）
BUTTONS = {
    "zh": ("下一步", "上一步", "完成"),
    "en": ("Next", "Previous", "Finish"),
    "ja": ("次へ", "前へ", "完了"),
    "ar": ("التالي", "السابق", "إنهاء"),
    "es": ("Siguiente", "Anterior", "Finalizar"),
    "fr": ("Suivant", "Précédent", "Terminer"),
    "de": ("Weiter", "Zurück", "Fertig"),
    "ru": ("Далее", "Назад", "Готово"),
    "pt": ("Seguinte", "Anterior", "Finalizar"),
}

# 用户菜单里的「重看本页引导」两条
LAYOUT_KEYS = {
    "zh": ("重看本页引导", "这一页还没有配置新手引导"),
    "en": ("Replay this page's tour", "No tour is configured for this page"),
    "ja": ("このページのガイドをもう一度", "このページにはまだガイドが設定されていません"),
    "ar": ("إعادة جولة هذه الصفحة", "لم تُعدَّ جولة لهذه الصفحة بعد"),
    "es": ("Repetir la guía de esta página", "Esta página todavía no tiene guía"),
    "fr": ("Revoir le guide de cette page", "Aucun guide n'est encore défini pour cette page"),
    "de": ("Tour dieser Seite wiederholen", "Für diese Seite ist noch kein Tour hinterlegt"),
    "ru": ("Повторить подсказку этой страницы", "Для этой страницы подсказка пока не настроена"),
    "pt": ("Repetir o guia desta página", "Esta página ainda não tem guia"),
}

# 自定义报表的维度/指标标签
CUSTOM_REPORT = {
    "zh": {
        "dimension": {"date": "日期", "source": "来源", "platform": "平台", "region": "地区", "userType": "用户类型"},
        "metric": {"count": "数量", "amount": "金额", "conversion": "转化率", "clickRate": "点击率", "openRate": "打开率"},
    },
    "en": {
        "dimension": {"date": "Date", "source": "Source", "platform": "Platform", "region": "Region", "userType": "User type"},
        "metric": {"count": "Count", "amount": "Amount", "conversion": "Conversion rate", "clickRate": "Click rate", "openRate": "Open rate"},
    },
    "ja": {
        "dimension": {"date": "日付", "source": "出典", "platform": "プラットフォーム", "region": "地域", "userType": "ユーザー種別"},
        "metric": {"count": "件数", "amount": "金額", "conversion": "コンバージョン率", "clickRate": "クリック率", "openRate": "開封率"},
    },
    "ar": {
        "dimension": {"date": "التاريخ", "source": "المصدر", "platform": "المنصة", "region": "المنطقة", "userType": "نوع المستخدم"},
        "metric": {"count": "العدد", "amount": "المبلغ", "conversion": "معدل التحويل", "clickRate": "معدل النقر", "openRate": "معدل الفتح"},
    },
    "es": {
        "dimension": {"date": "Fecha", "source": "Origen", "platform": "Plataforma", "region": "Región", "userType": "Tipo de usuario"},
        "metric": {"count": "Cantidad", "amount": "Importe", "conversion": "Tasa de conversión", "clickRate": "Tasa de clics", "openRate": "Tasa de aperturas"},
    },
    "fr": {
        "dimension": {"date": "Date", "source": "Source", "platform": "Plateforme", "region": "Région", "userType": "Type d'utilisateur"},
        "metric": {"count": "Nombre", "amount": "Montant", "conversion": "Taux de conversion", "clickRate": "Taux de clics", "openRate": "Taux d'ouverture"},
    },
    "de": {
        "dimension": {"date": "Datum", "source": "Quelle", "platform": "Plattform", "region": "Region", "userType": "Benutzertyp"},
        "metric": {"count": "Anzahl", "amount": "Betrag", "conversion": "Conversion-Rate", "clickRate": "Klickrate", "openRate": "Öffnungsrate"},
    },
    "ru": {
        "dimension": {"date": "Дата", "source": "Источник", "platform": "Платформа", "region": "Регион", "userType": "Тип пользователя"},
        "metric": {"count": "Количество", "amount": "Сумма", "conversion": "Конверсия", "clickRate": "CTR", "openRate": "Open rate"},
    },
    "pt": {
        "dimension": {"date": "Data", "source": "Origem", "platform": "Plataforma", "region": "Região", "userType": "Tipo de utilizador"},
        "metric": {"count": "Quantidade", "amount": "Valor", "conversion": "Taxa de conversão", "clickRate": "Taxa de cliques", "openRate": "Taxa de abertura"},
    },
}


def esc(s):
    return json.dumps(s, ensure_ascii=False)


def find_obj(lines, header, indent):
    """定位一个以 header 开头的对象，返回 (start, end) 行号（0 基、含两端）。"""
    start = None
    for i, l in enumerate(lines):
        if l.strip().startswith(header) and l.startswith(indent):
            start = i
            break
    if start is None:
        raise SystemExit(f"找不到 {header!r}")
    depth = 0
    for i in range(start, len(lines)):
        depth += lines[i].count("{") - lines[i].count("}")
        if depth == 0:
            return start, i
    raise SystemExit(f"{header!r} 花括号不配平")


def build_onboarding_block(lang, g, pages):
    """生成 setup.onboarding 整块。"""
    nxt, prv, done = BUTTONS[lang]
    L = ['    "onboarding": {']
    L.append(f'      "next": {esc(nxt)},')
    L.append(f'      "prev": {esc(prv)},')
    L.append(f'      "done": {esc(done)},')
    L.append(f'      "close": {esc(g["close"])},')
    L.append('      "global": {')
    for i in range(1, 7):
        s = g["global"]["step%d" % i]
        L.append(f'        "step{i}": {{')
        L.append(f'          "title": {esc(s["title"])},')
        L.append(f'          "description": {esc(s["description"])}')
        L.append('        }' + (',' if i < 6 else ''))
    L.append('      },')
    L.append('      "page": {')
    L.append('        "common": {')
    L.append(f'          "overview": {esc(g["overview"])},')
    # 第 2 步的标题是共享的：各页措辞一样，具体讲什么在 description 里。
    # 共享键让每页只新增 2 条词条（summary + step2.description），
    # 9 个语言 × 200 多页的词条量才压得住。
    L.append(f'          "action": {esc(g["action"])}')
    L.append('        },')
    L.append('        "messageHubList": {')
    L.append(f'          "summary": {esc(g["summary"])},')
    L.append('          "step2": {')
    L.append(f'            "title": {esc(g["step2Title"])},')
    L.append(f'            "description": {esc(g["step2Description"])}')
    L.append('          }')
    L.append('        },')
    keys = list(pages.keys())
    for i, pk in enumerate(keys):
        d = pages[pk]
        L.append(f'        "{pk}": {{')
        L.append(f'          "summary": {esc(d["summary"])},')
        step_keys = [k for k in d if k != "summary"]
        for j, sk in enumerate(step_keys):
            L.append(f'          "{sk}": {{')
            # step 的 title 可以缺省：第 2 步的标题改用共享键
            # setup.onboarding.page.common.action（各页措辞一样，
            # 真正讲清这一页是什么的是 description）。
            if "title" in d[sk]:
                L.append(f'            "title": {esc(d[sk]["title"])},')
            L.append(f'            "description": {esc(d[sk]["description"])}')
            L.append('          }' + (',' if j < len(step_keys) - 1 else ''))
        L.append('        }' + (',' if i < len(keys) - 1 else ''))
    L.append('      }')
    L.append('    }')
    return L


def build_custom_report_block(lang):
    d = CUSTOM_REPORT[lang]
    L = ['  "customReport": {']
    L.append('    "dimension": {')
    dim = list(d["dimension"].items())
    for i, (k, v) in enumerate(dim):
        L.append(f'      "{k}": {esc(v)}' + (',' if i < len(dim) - 1 else ''))
    L.append('    },')
    L.append('    "metric": {')
    met = list(d["metric"].items())
    for i, (k, v) in enumerate(met):
        L.append(f'      "{k}": {esc(v)}' + (',' if i < len(met) - 1 else ''))
    L.append('    }')
    L.append('  },')
    return L


def main():
    if len(sys.argv) < 2:
        raise SystemExit(__doc__)
    with open(sys.argv[1], encoding="utf-8") as fh:
        gdata = json.load(fh)
    pages = {}
    for path in sys.argv[2:]:
        with open(path, encoding="utf-8") as fh:
            b = json.load(fh)
        for lang, pl in b.items():
            pages.setdefault(lang, {}).update(pl)

    for lang in LANGS:
        g = gdata[lang]
        pl = pages.get(lang, {})
        p = os.path.join(LOCALES_DIR, f"{lang}.json")
        with open(p, encoding="utf-8") as fh:
            lines = fh.read().split("\n")

        # 1) 替换 setup.onboarding 整块
        s, e = find_obj(lines, '"onboarding": {', "    ")
        lines = lines[:s] + build_onboarding_block(lang, g, pl) + lines[e + 1:]

        # 2) layout 下补两条（globalTour 之后）——已存在就跳过（脚本要可重跑）
        if '"pageTour":' not in "\n".join(lines):
            gi = next(i for i, l in enumerate(lines) if l.strip().startswith('"globalTour"'))
            if not lines[gi].rstrip().endswith(","):
                lines[gi] = lines[gi].rstrip() + ","
            tour, unavail = LAYOUT_KEYS[lang]
            lines = lines[:gi + 1] + [
                f'    "pageTour": {esc(tour)},',
                f'    "pageTourUnavailable": {esc(unavail)}',
            ] + lines[gi + 1:]

        # 3) 插入 customReport 命名空间（放在 "setup" 之前）
        if '"customReport": {' not in "\n".join(lines):
            si = next(i for i, l in enumerate(lines) if l.strip().startswith('"setup": {'))
            lines = lines[:si] + build_custom_report_block(lang) + lines[si:]

        with open(p, "w", encoding="utf-8") as fh:
            fh.write("\n".join(lines))

        # 4) 删掉已无消费方的中文字面量键「来源」
        with open(p, encoding="utf-8") as fh:
            txt = fh.read()
        txt = "\n".join(
            l for l in txt.split("\n")
            if not l.strip().startswith('"来源":')
        )
        with open(p, "w", encoding="utf-8") as fh:
            fh.write(txt)

        with open(p, encoding="utf-8") as fh:
            data = json.load(fh)
        ob = data["setup"]["onboarding"]
        assert ob["close"] == g["close"], lang
        assert set(pl.keys()) <= set(ob["page"].keys()), lang
        assert "customReport" in data, lang
        print(f"{lang}: onboarding(global+{len(pl)} 页面) / layout.pageTour / customReport 就绪")


if __name__ == "__main__":
    main()
