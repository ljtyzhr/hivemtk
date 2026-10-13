#!/usr/bin/env python3
"""核对错误处理三份文档里出现的码值字面量，逐个回代码登记表对账。

这一族缺陷的根因是「文档抄了一份代码里并不存在的码清单」，因此判据必须能区分两类字符串：

  - 真码值（`CodeParamInvalid = 4000`、`UNKNOWN_1000`）：必须在登记表里存在，且值要对得上；
  - 占位形状（`CodeXxx`、`bizerr.CodeXxx`）：写成用法签名用的通配，含 `Xxx` 即判为占位，不参与对账。

不做常驻门：本仓的静态门链（`make audit` 与 user-server-ci 的 static-gates）由门禁泳道维护，
新增一步要同时改 Makefile、workflow 两份 paths 镜像与 `check-ci-gate-paths.py` 的站点计数。
`/speckit-bug-assess` 对本缺陷预设的退化方案是「文档只引用文件路径、不列码值」，本轮按该方案落地，
此脚本作为该退化方案的可复现验证：规范文档里不再出现任何**未登记**的码值字面量。

用法：python3 .specify/bugs/business-error-code-panic/check-doc-code-literals.py
退出码：0 = 全部对得上；1 = 有码值找不到定义或与代码不符。
"""
import re
import sys
from pathlib import Path

# 文档坐标：本脚本在 <repo>/.specify/bugs/<slug>/ 下，parents[3] 才是仓库根（hivemtk/）。
REPO = Path(__file__).resolve().parents[3]
NUMERIC_REGISTRY = REPO / "user-server/internal/domain/errors/codes.go"
STRING_REGISTRY = REPO / "user-server/internal/pkg/utils/error_code.go"
DOCS = [
    REPO / "CLAUDE.md",
    REPO / "docs/architecture/adr/ADR-009-error-handling.md",
]


def load_numeric():
    src = NUMERIC_REGISTRY.read_text(encoding="utf-8")
    return {m.group(1): int(m.group(2))
            for m in re.finditer(r"^\t(\w+)\s*=\s*(\d+)\s*$", src, re.M)}


def load_string():
    src = STRING_REGISTRY.read_text(encoding="utf-8")
    return {m.group(2): m.group(1)
            for m in re.finditer(r'^\t(\w+)\s+ErrorCode\s*=\s*"([^"]+)"', src, re.M)}


def main():
    numeric = load_numeric()
    string_codes = load_string()
    print(f"登记表读数：数值 {len(numeric)} 条 / 字符串 {len(string_codes)} 条")

    problems = []
    checked = 0
    for doc in DOCS:
        text = doc.read_text(encoding="utf-8")
        rel = doc.relative_to(REPO)

        for ident in sorted(set(re.findall(r"\b(Code[A-Z]\w*)\b", text))):
            if "Xxx" in ident:
                continue
            checked += 1
            if ident not in numeric:
                problems.append(f"{rel}: 引用常量 `{ident}`，但 codes.go 里没有这一条")

        for ident, value in re.findall(r"\b(\w+)\s*=\s*(\d{1,5})\b", text):
            if ident not in numeric:
                continue
            checked += 1
            if numeric[ident] != int(value):
                problems.append(
                    f"{rel}: 文档写 `{ident} = {value}`，codes.go 实际是 {numeric[ident]}"
                )

        for literal in sorted(set(re.findall(r"\b([A-Z][A-Z_0-9]*_\d{4})\b", text))):
            if literal.endswith("_0000") or "Xxx" in literal:
                continue
            checked += 1
            if literal not in string_codes:
                problems.append(
                    f"{rel}: 引用字符串码 `{literal}`，error_code.go 的码表里没有这一条"
                )

    for p in problems:
        print("MISMATCH " + p)
    print(f"对账项 {checked} 处")
    if problems:
        print(f"FAIL：{len(problems)} 处文档码值与代码登记表不符")
        return 1
    print("OK：文档里出现的每一个码值字面量都能在代码登记表里找到，且取值一致")
    return 0


if __name__ == "__main__":
    sys.exit(main())
