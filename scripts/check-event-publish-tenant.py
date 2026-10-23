#!/usr/bin/env python3
"""事件发布租户门：租户域事件**只能**经 `event.PublishCtx` / `event.PublishEventCtx` 发布。

为什么需要这道门（2026-10-23 · SaaS 全链路审计 A9）：

T021 的落地形态是「每个发布点自己 `TenantID: tenantIDFromCtx(ctx)`」。当时全仓 4 个发布点
都写对了，但那是**约定**不是**结构**：新增一个发布点、照抄漏了那行，后果是订阅者拿到 0，
on 模式下要么被隔离 callback fail-closed（静默空转、只报数不报错），要么落默认租户（错租户写数据）。
同一个失效模式在本仓已复现过两次（WS 广播、bridge 回扫 cron）。

判据（不猜实现，与 check-gorm-only.sh 同一口径：把执行者收成唯一一个）：

  `user-server/internal/**` 里除 `internal/event/` 之外，**禁止**
    - `event.Publish(`        —— 裸发布口，租户要调用方自己填；
    - `.Publish(event.Event{` —— 绕过便捷函数、直接塞 Event 给总线。
  必须改用 `event.PublishCtx(ctx, topic, payload)` / `event.PublishEventCtx(ctx, evt)`：
  两个函数会把 ctx 里的租户盖章进载荷的 `TenantID` 字段（已有非 0 值不覆盖）。

豁免：
  - `internal/event/` 自身（实现方）；
  - `*_test.go`（用例需要构造裸事件来验证总线行为）。

执行：`python3 scripts/check-event-publish-tenant.py`（Makefile `audit` 目标与
`.github/workflows/user-server-ci.yml` 的 static-gates 作业各有一处调用）。
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
# 允许用环境变量改扫描根：仅用于**给这道门自己做牙齿测试**（在 /tmp 造一个违规样本，
# 不碰真实源码树——往真树里塞违规样本会被并行泳道的编译看到）。
SCAN_ROOT = Path(os.environ.get("EVENT_PUBLISH_SCAN_ROOT") or (REPO / "user-server" / "internal"))
SELF_PKG = (REPO / "user-server" / "internal" / "event").resolve()

# 违规形状：裸发布口 / 直塞 Event 给总线
FORBIDDEN = (
    (re.compile(r"\bevent\.Publish\("), "event.Publish(", "event.PublishCtx(ctx, topic, payload)"),
    (re.compile(r"\.Publish\(event\.Event\{"), ".Publish(event.Event{", "event.PublishEventCtx(ctx, evt)"),
)


def iter_go_sources():
    for path in sorted(SCAN_ROOT.rglob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        if path.resolve().is_relative_to(SELF_PKG):
            continue
        if any(part in ("vendor", "node_modules") for part in path.parts):
            continue
        yield path


def rel_path(path: Path) -> str:
    """相对路径显示：树外（牙齿测试的 /tmp 样本）就退化为绝对路径。"""
    try:
        return path.relative_to(REPO).as_posix()
    except ValueError:
        return str(path)


def scan() -> list[str]:
    problems: list[str] = []
    for path in iter_go_sources():
        rel = rel_path(path)
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError as exc:
            problems.append("%s 读取失败：%s" % (rel, exc))
            continue
        for lineno, line in enumerate(text.splitlines(), 1):
            stripped = line.lstrip()
            if stripped.startswith("//"):  # 注释里提到不算
                continue
            for regex, shape, fix in FORBIDDEN:
                if regex.search(line):
                    problems.append(
                        "%s:%d 使用了 %s（租户不会被盖章）—— 改用 %s" % (rel, lineno, shape, fix)
                    )
    return problems


def main() -> int:
    if not SCAN_ROOT.is_dir():
        print("✗ 扫描根不存在：%s（rc=2：区分「没扫」与「零命中」）" % SCAN_ROOT, file=sys.stderr)
        return 2
    problems = scan()
    if problems:
        print("── 事件发布租户门（FAILED）──")
        for p in problems:
            print("  " + p)
        print()
        print("判据说明见脚本头注释；盖章函数：user-server/internal/event/bus.go 的 StampTenantFromCtx。")
        return 1
    scanned = sum(1 for _ in iter_go_sources())
    print("OK event-publish-tenant：%d 个文件，未发现裸发布口（租户统一由 PublishCtx 盖章）" % scanned)
    return 0


if __name__ == "__main__":
    sys.exit(main())
