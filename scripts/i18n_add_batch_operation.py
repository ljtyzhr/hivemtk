#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""给「批量运营」页补一套 batchOperation.* 点分词条，并清掉因此失去消费方的中文字面量键。

背景：批量运营页原先用 `i18n.global.t('导入完成')` 这种**中文字面量键**取文案。
i18n 棘轮门把「中文字面量直传」当劣化项（raw_t_calls 越大越差），这类写法既
在门里挂账、又躲开了「点分键才能被覆盖率门与前端提取器认出来」的路径。

做法：
  1. 9 个语言包各加一个 batchOperation 命名空间（点分键）；
  2. 页面里改成 t('batchOperation.xxx')；
  3. 删掉因此没有消费方的中文字面量键（批量任务已提交 / 执行失败 / 取消失败）。

「已取消」**不删**：reachPipeline/List.vue 也在用它。
"""
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LOCALES_DIR = os.path.join(ROOT, "user-web", "src", "i18n", "locales")
LANGS = ["zh", "en", "ja", "ar", "es", "fr", "de", "ru", "pt"]

# batchOperation 命名空间：每个语言一套
BATCH_OP = {
    "zh": {
        "importDone": "导入完成",
        "importFailed": "导入失败",
        "taskSubmitted": "批量任务已提交",
        "executeFailed": "执行失败",
        "cancelFailed": "取消失败",
        "running": "正在执行批量操作...",
    },
    "en": {
        "importDone": "Import completed",
        "importFailed": "Import failed",
        "taskSubmitted": "Batch task submitted",
        "executeFailed": "Execution failed",
        "cancelFailed": "Failed to cancel",
        "running": "Running the batch operation...",
    },
    "ja": {
        "importDone": "インポートが完了しました",
        "importFailed": "インポートに失敗しました",
        "taskSubmitted": "バッチタスクが送信されました",
        "executeFailed": "実行に失敗しました",
        "cancelFailed": "キャンセルできませんでした",
        "running": "一括操作を実行しています...",
    },
    "ar": {
        "importDone": "اكتمل الاستيراد",
        "importFailed": "فشل الاستيراد",
        "taskSubmitted": "تم إرسال مهمة الدفعة",
        "executeFailed": "فشل التنفيذ",
        "cancelFailed": "تعذر الإلغاء",
        "running": "جارٍ إجراء عملية مجمعة...",
    },
    "es": {
        "importDone": "Importación completada",
        "importFailed": "Error al importar",
        "taskSubmitted": "Tarea por lotes enviada",
        "executeFailed": "Error de ejecución",
        "cancelFailed": "No se pudo cancelar",
        "running": "Ejecutando la operación por lotes...",
    },
    "fr": {
        "importDone": "Importation terminée",
        "importFailed": "Échec de l'importation",
        "taskSubmitted": "Tâche par lots envoyée",
        "executeFailed": "Échec de l'exécution",
        "cancelFailed": "Impossible d'annuler",
        "running": "Exécution de l'opération par lots...",
    },
    "de": {
        "importDone": "Import abgeschlossen",
        "importFailed": "Import fehlgeschlagen",
        "taskSubmitted": "Stapelaufgabe übermittelt",
        "executeFailed": "Ausführung fehlgeschlagen",
        "cancelFailed": "Abbrechen fehlgeschlagen",
        "running": "Stapelverarbeitung wird ausgeführt...",
    },
    "ru": {
        "importDone": "Импорт завершён",
        "importFailed": "Не удалось импортировать",
        "taskSubmitted": "Пакетная задача отправлена",
        "executeFailed": "Ошибка выполнения",
        "cancelFailed": "Не удалось отменить",
        "running": "Выполняется пакетная операция...",
    },
    "pt": {
        "importDone": "Importação concluída",
        "importFailed": "Falha na importação",
        "taskSubmitted": "Tarefa em lote enviada",
        "executeFailed": "Falha na execução",
        "cancelFailed": "Não foi possível cancelar",
        "running": "Executando a operação em lote...",
    },
}

# 页面里要改的点分键映射：旧中文字面量 -> 新点分键
SITE = {
    "导入完成": "batchOperation.importDone",
    "导入失败": "batchOperation.importFailed",
    "批量任务已提交": "batchOperation.taskSubmitted",
    "执行失败": "batchOperation.executeFailed",
    "取消失败": "batchOperation.cancelFailed",
    "正在执行批量操作...": "batchOperation.running",
}

# 改完之后失去消费方的中文字面量键（要从语言包里删掉）
DROP = ["批量任务已提交", "执行失败", "取消失败", "正在执行批量操作..."]


def esc(s):
    return json.dumps(s, ensure_ascii=False)


def build_block(lang):
    d = BATCH_OP[lang]
    L = ['  "batchOperation": {']
    items = list(d.items())
    for i, (k, v) in enumerate(items):
        L.append(f'    "{k}": {esc(v)}' + ("," if i < len(items) - 1 else ""))
    L.append("  },")
    return L


def main():
    for lang in LANGS:
        p = os.path.join(LOCALES_DIR, f"{lang}.json")
        with open(p, encoding="utf-8") as fh:
            lines = fh.read().split("\n")

        # 1) 插入 batchOperation 命名空间（放在 "setup" 之前）
        if '"batchOperation": {' not in "\n".join(lines):
            si = next(i for i, l in enumerate(lines) if l.strip().startswith('"setup": {'))
            lines = lines[:si] + build_block(lang) + lines[si:]

        # 2) 删掉失去消费方的中文字面量键
        text = "\n".join(lines)
        for key in DROP:
            text = re.sub(r'\n  "%s":\s*"(?:[^"\\]|\\.)*",?' % re.escape(key), "", text)
        lines = text.split("\n")

        with open(p, "w", encoding="utf-8") as fh:
            fh.write("\n".join(lines))

        data = json.load(open(p, encoding="utf-8"))
        assert set(BATCH_OP[lang]) <= set(data["batchOperation"]), lang
        print(f"{lang}: batchOperation 命名空间就绪（{len(BATCH_OP[lang])} 条），"
              f"已清 {len(DROP)} 条失去消费方的字面量键")

    # 3) 改页面里的取词方式（幂等：已改过的就跳过替换）
    vue = os.path.join(ROOT, "user-web", "src", "views", "batchOperation", "List.vue")
    with open(vue, encoding="utf-8") as fh:
        src = fh.read()
    for old, new in SITE.items():
        src = src.replace("i18n.global.t('%s')" % old, "i18n.global.t('%s')" % new)
    # '导入失败：' + err  —— 冒号单独拼字面量也一起收进词条
    src = src.replace(
        "ElMessage.error('导入失败：' + (err?.message || err))",
        "ElMessage.error(i18n.global.t('batchOperation.importFailed') + '：' + (err?.message || err))",
    )
    with open(vue, "w", encoding="utf-8") as fh:
        fh.write(src)
    for old, new in SITE.items():
        assert "i18n.global.t('%s')" % new in src, f"页面没改到 {new}"
        assert "i18n.global.t('%s')" % old not in src, f"页面还留着字面量键 {old}"
    print("batchOperation/List.vue: 取词方式已改为点分键")


if __name__ == "__main__":
    main()
