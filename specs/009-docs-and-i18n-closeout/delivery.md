# Delivery Report: F1 文档分层 + F3 国际化收口 + C4 多租户文档口径

**Feature**: `009-docs-and-i18n-closeout` ｜ **交付日期**: 2026-10-24 ｜ **Spec/Plan/Tasks**: 同目录

## 一句话

三条 P2 文档/口径收口以「**新增导航 + 新增口径/复核文档 + 对干净文件增量勘误**」交付：
F1 补齐 4 个缺失的分层导航页；F3 产出后端文案国际化口径与 127 项缺口清单（前端 JSON 门禁实测已 100% 绿，无需补）；
C4 产出多租户实现 vs 文档复核，并勘误 2 处陈旧硬约束/头句。

## 改动文件清单

### 新增（hivemtk 仓）

| 文件 | 归属 | 说明 |
|---|---|---|
| `hivemtk/specs/009-docs-and-i18n-closeout/spec.md` | 009 | 规格：目标/取证/Requirements/验收 |
| `hivemtk/specs/009-docs-and-i18n-closeout/plan.md` | 009 | 计划：分层目标态、执行顺序、决策点 |
| `hivemtk/specs/009-docs-and-i18n-closeout/tasks.md` | 009 | 任务清单（T001–T018，全部 [x]） |
| `hivemtk/specs/009-docs-and-i18n-closeout/delivery.md` | 009 | 本文件 |
| `hivemtk/specs/README.md` | F1 | spec 施工源索引（001/002/005/006/009；007/012 lane 在途只留注） |
| `hivemtk/docs/architecture/README.md` | F1 | 架构层导航（总览/深度解析/多租户与部署形态/模块专题/ADR） |
| `hivemtk/docs/operations/README.md` | F1 | 运维层导航（部署初始化/运行保障/功能专题/反代模板） |
| `hivemtk/docs/standards/README.md` | F1 | 标准与口径层导航 |
| `hivemtk/docs/standards/BACKEND_MESSAGE_I18N_POLICY.md` | F3 | 后端文案国际化口径与清单（三类口径 + 184/100/127 读数 + 127 项缺口清单 + 决策点） |
| `hivemtk/docs/architecture/TENANT_MODE_REVIEW.md` | C4 | 多租户实现 vs 文档逐条核对 + 授权策略复核 + 勘误清单 |

### 修改（干净文件，增量编辑）

| 文件 | 改法 |
|---|---|
| `hivemtk/docs/INDEX.md` | ① 头句「单商户本地部署」→ 双形态（off/on）；② 挂 4 层导航 + TENANT_MODE_REVIEW + SAAS_PRODUCTION_CONSTRAINTS；③ 补 005/006/009 规格施工源；④ 更新最后更新行 |
| `hivemtk/docs/DEV_DOCS_INDEX.md` | E4 硬约束勘误：「业务表无需多租户字段」→「245 张业务表带 tenant_id、12 张全局表豁免」，权威来源挂 TENANT_MODE_REVIEW.md |
| `docs/INDEX.md`（workspace 顶层，私有仓） | 「增量补录」节追加 009 泳道 7 条新文档（只追加，不重排计数） |

> 未触碰：Go 源码、前端 9 语种 locale JSON、`specs/006*/**`、`CHANGELOG.md`、lane 在途文件（含未提交的 `specs/007-*/`、`specs/012-*/`）。

## 检查命令与结果（本机实测）

| 检查 | 命令 | 结果 |
|---|---|---|
| i18n 覆盖率门禁 | `cd hivemtk/user-web && node scripts/check-i18n-coverage.cjs` | ✅ rc=0，9 语种 1978 key 全 100%，0 缺失 0 空值 |
| i18n 硬编码棘轮 | `node scripts/check-i18n-hardcode.cjs` | ✅ rc=0（10460 行 / 220 文件，与基线一致） |
| i18n bash 检查 | `bash scripts/check-i18n-coverage.sh` | ✅ rc=0（总缺失 0 ≤ 5） |
| `$t()` 引用核对 | 源码扫描 | ✅ 730 个去重 key，used-but-missing = 0 |
| 后端文案覆盖 | 源码扫描 | 184 种中文响应消息 / BackendPhrases 覆盖 100 / 缺口 127 |
| 相对链接校验 | 对本 lane 新增/修改的 10 个 md 逐链接核存在性 | ✅ 全部目标存在 |
| 全仓链接核（参考） | `python3 scripts/md_path_check.py` | ⚠️ 全仓 1530 处 GHOST，绝大多数位于 `.tmp_files/` 在途快照与既有文档，非本 lane 引入；故本 lane 改用针对性链接校验 |

未跑：Go 测试（铁律不要求）；前端 `npm run build`（node_modules 风险由主人判断，本 lane 不跑，如实说明）。

## 调研取证（写进 spec.md 的证据）

1. **F1**：`hivemtk/docs/` 145 篇 `.md`，顶层 INDEX 未收录 76 篇（约 47% 覆盖率）；architecture/operations/standards 三目录与 `specs/` 均无层内导航。
2. **F3**：前端 i18n 门禁 100% 绿、无缺失 key；后端已具 `internal/pkg/i18n` 四语设施，缺口是词典覆盖（127 种中文字面消息未收录）与语言集合（后端 4 语 vs 前端 9 语）。
3. **C4**：实现与 spec 目标口径一致；4 份早期文档停在「单商户时代」：DEV_DOCS_INDEX E4、hivemtk/docs/INDEX 头句、DATABASE_SCHEMA_DEEP_DIVE §3.1/X3、research D6 命名漂移。

## 遗留 / 决策点

| # | 项 | 状态 | 建议 |
|---|---|---|---|
| 1 | 后端 127 种未覆盖中文消息 | 只列清单未施工 | 后续 lane 按 `BACKEND_MESSAGE_I18N_POLICY.md` §四分域补 `BackendPhrases` 或迁移 `i18n.T()` 语义 key（本 lane 铁律不写 Go） |
| 2 | 后端 4 语 vs 前端 9 语 | 决策点 | 建议词典先维持 4 语，扩 9 语单独立项并配 CI 门禁 |
| 3 | 前端 9 语种 JSON | 在途（并发 lane `M`） | 本 lane 未触碰；门禁已绿，无补 key 需求 |
| 4 | `DATABASE_SCHEMA_DEEP_DIVE.md` §3.1/X3 勘误 | 只登记（TENANT_MODE_REVIEW §四-3） | 该大体量文件可能 lane 在途，待其稳定后由主人整写订正 |
| 5 | 顶层 `docs/INDEX.md` 计数（339） | 只追加增量补录节 | 重排计数与 `doc_inventory.json` 同步超出本 lane 风险预算，建议 workspace 仓后续对账 |
| 6 | `specs/007-*`、`specs/012-*` 未提交目录 | 并发 lane 在途 | 未登记入 specs/README（只留注），待各自 delivery 收口补录 |
| 7 | `WithSystemScope` 约 95 处调用 | 已知能力耦合 | 沿用 SAAS_MULTI_TENANT_EVALUATION §8 方案，待批准后走 spec-kit 施工，本 lane 不重复展开 |

## 验收对照

- SC-001 ✅ 新增导航页 5 篇，全部被 `hivemtk/docs/INDEX.md` 或顶层 `docs/INDEX.md` 引用。
- SC-002 ✅ 三个 i18n 门禁 rc=0，前端 key 数 1978 不降。
- SC-003 ✅ C4 复核文档列出全部出入项，并对 2 处干净文件完成勘误。
- SC-004 ✅ 四件套齐备，本文件写清改动文件、检查命令与结果、遗留项。
