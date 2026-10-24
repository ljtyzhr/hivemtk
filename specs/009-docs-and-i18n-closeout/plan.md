# Implementation Plan: F1 文档分层 + F3 国际化收口 + C4 多租户文档口径

**Branch**: `009-docs-and-i18n-closeout` | **Date**: 2026-10-24 | **Spec**: [spec.md](./spec.md)

## Summary

纯文档/口径 lane：不写 Go、不改前端 JSON、不移动 lane 在途文件。以「新增导航页 + 新增口径/复核文档 + 对干净文件做增量勘误」完成三条 P2 收口。

## Technical Context

- **类型**: 文档治理（documentation governance）
- **事实源**: `hivemtk/docs/`（145 篇 .md）、`hivemtk/specs/001-saas-multi-tenant/`、`user-server/internal/pkg/tenant/`、`user-server/internal/pkg/i18n/`、`user-web/scripts/check-i18n-*`
- **约束（铁律）**: 不 git add/commit/push；不触碰 Go 源码、前端 locale JSON、`specs/006*/**`、`CHANGELOG.md` 等他人所有文件；`docs/architecture/` 下优先新增不整写
- **验证**: i18n 门禁自检（`node scripts/check-i18n-coverage.cjs`、`node scripts/check-i18n-hardcode.cjs`、`bash scripts/check-i18n-coverage.sh`）均须 rc=0；不跑 Go 测试

## 分层结构（目标态）

```
docs/INDEX.md（顶层唯一入口，全仓）
├── hivemtk/docs/INDEX.md（用户端子索引）            ← 本 lane 增量挂导航
│   ├── docs/architecture/README.md（架构层导航）     ← 新增
│   ├── docs/operations/README.md（运维层导航）       ← 新增
│   ├── docs/standards/README.md（标准层导航）        ← 新增
│   │   └── docs/standards/BACKEND_MESSAGE_I18N_POLICY.md ← 新增（F3 口径+清单）
│   ├── docs/handbook/README.md（手册四册入口）       ← 既有
│   └── docs/DEV_DOCS_INDEX.md（工程开发文档索引）    ← E4 勘误
├── hivemtk/docs/architecture/TENANT_MODE_REVIEW.md  ← 新增（C4 复核）
└── hivemtk/specs/README.md（spec 施工源索引）        ← 新增
```

## 执行顺序

1. **取证**（已完成，结论入 spec.md）：文档扫描、i18n 门禁实跑、后端文案扫描、tenant 代码比对。
2. **F1 导航页**：architecture / operations / standards / specs 四篇 README。
3. **F3 口径文档**：`BACKEND_MESSAGE_I18N_POLICY.md`（口径三类 + 127 项未覆盖清单）。
4. **C4 复核文档**：`TENANT_MODE_REVIEW.md`（核对 + 勘误 + 授权策略）。
5. **增量挂载**：`hivemtk/docs/INDEX.md` 挂导航、修头句；`DEV_DOCS_INDEX.md` 修 E4；顶层 `docs/INDEX.md` 增量补录。
6. **验收**：门禁自检 rc=0；git status 核对只新增/修改了职责内文件。

## Risks / 决策点

| 决策点 | 选择 | 理由 |
|---|---|---|
| 前端 JSON 补 key | **不补** | 门禁已 100% 绿（0 缺失 0 空值），且 9 语种 JSON 正被并发 lane 修改；补了反而冲突 |
| 后端未覆盖文案（127 种） | **只列清单不写 Go** | 铁律禁改 Go 源码；把 127 项清单与语义 key 建议写进标准文档，供后续 lane 施工 |
| `DATABASE_SCHEMA_DEEP_DIVE.md` §3.1 勘误 | **新文档登记、不整写** | 大体量文件可能 lane 在途；在 TENANT_MODE_REVIEW.md 留勘误条目即可 |
| 顶层 `docs/INDEX.md` 计数 | **只追加增量补录节** | 该文件头部计数为 2026-10-23 实算值，重排计数超出本 lane 风险预算 |
