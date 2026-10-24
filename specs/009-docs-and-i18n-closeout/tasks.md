---
description: "任务清单：F1 文档分层 + F3 国际化收口 + C4 多租户文档口径"
---

# Tasks: F1 文档分层 + F3 国际化收口 + C4 多租户文档口径

**Input**: Design documents from `/specs/009-docs-and-i18n-closeout/`

**Prerequisites**: spec.md ✅ | plan.md ✅

**约束**: 本 lane 不写 Go、不改前端 JSON、不移动 lane 在途文件；只新增文档 + 对干净文件增量勘误。

---

## Phase 1: 调研取证

- [x] T001 扫描 `hivemtk/docs/` 全量 `.md`（145 篇），与顶层 `docs/INDEX.md` 比对得出未收录 76 篇；列出 `docs/architecture/`、`docs/operations/`、`docs/standards/`、`hivemtk/specs/` 缺索引的现状
- [x] T002 实跑前端 i18n 门禁：`check-i18n-coverage.cjs`（rc=0，9 语种 1978 key 全 100%）、`check-i18n-coverage.sh`（rc=0）、`check-i18n-hardcode.cjs`（rc=0，10460 行基线一致）；扫描 `$t()` 引用 used-but-missing = 0
- [x] T003 读 `user-server/internal/pkg/i18n/` 全套（messages/backend_phrases/lang_ctx/lang_detect/locale）与 `middleware/locale.go`、`response.go`，统计后端 `response.*` 中文消息 184 种、`BackendPhrases` 覆盖 100、未覆盖 127
- [x] T004 读 `specs/001-saas-multi-tenant/` 全套与 `internal/pkg/tenant/*`、`internal/pkg/tenancyidx/*`、`cmd/api/main.go`、`middleware/tenant.go`，逐条比对文档口径与实现，产出出入清单

## Phase 2: F1 文档分层

- [x] T005 新增 `hivemtk/docs/architecture/README.md`：按「总览 / 深度解析 / 多租户与部署形态 / 模块与专题 / ADR」分层登记 `docs/architecture/` 全部 `.md`
- [x] T006 新增 `hivemtk/docs/operations/README.md`：登记 `docs/operations/` 全部文档（含 EPAY_LIVE_CHECKLIST 等顶层 INDEX 未收录条目）与反代模板
- [x] T007 新增 `hivemtk/docs/standards/README.md`：登记 `docs/standards/`（MASTER_RULES / FEATURE_DOCUMENTATION_TEMPLATE / BACKEND_MESSAGE_I18N_POLICY）
- [x] T008 新增 `hivemtk/specs/README.md`：登记 spec 施工源 001/002/005/006/009 与「spec 唯一施工源」口径

## Phase 3: F3 国际化收口

- [x] T009 新增 `hivemtk/docs/standards/BACKEND_MESSAGE_I18N_POLICY.md`：三类口径（必须 i18n / 不 i18n / 改造顺序）+ 后端文案清单（184/100/127）+ 前端门禁现状证据
- [x] T010 复核前端 JSON：确认门禁 rc=0、key 数 1978 不降；因 9 语种 JSON 在途，本 lane 不改 JSON，仅登记证据

## Phase 4: C4 多租户文档口径复核

- [x] T011 新增 `hivemtk/docs/architecture/TENANT_MODE_REVIEW.md`：实现 vs 文档逐条核对（模式解析/默认租户/隔离 callback/Fanout/键分域/行自证/授权策略）
- [x] T012 勘误 `hivemtk/docs/INDEX.md` 头句「单商户本地部署（私域模式）」→ 双形态
- [x] T013 勘误 `hivemtk/docs/DEV_DOCS_INDEX.md` E4「业务表无需多租户字段」→ 001 后 245 表带 tenant_id、12 张全局表豁免；权威来源挂入 TENANT_MODE_REVIEW.md
- [x] T014 登记 `DATABASE_SCHEMA_DEEP_DIVE.md` §3.1/X3 勘误条目（只登记，不整写）

## Phase 5: 挂载与验收

- [x] T015 编辑 `hivemtk/docs/INDEX.md`：挂 architecture/operations/standards/specs 导航与 BACKEND_MESSAGE_I18N_POLICY、TENANT_MODE_REVIEW、SAAS_PRODUCTION_CONSTRAINTS、specs 005/006/009
- [x] T016 编辑顶层 `docs/INDEX.md`：在「增量补录」节追加本 lane 新增文档条目（只追加）
- [x] T017 验收：复跑三个 i18n 门禁 rc=0；`git status --porcelain` 核对改动仅限职责内文件
- [x] T018 撰写 `specs/009-docs-and-i18n-closeout/delivery.md`（改动文件、检查命令与结果、遗留项）

---

## Dependencies

T001–T004（取证）→ T005–T008（F1）∥ T009–T010（F3）∥ T011–T014（C4）→ T015–T016（挂载）→ T017–T018（验收）
