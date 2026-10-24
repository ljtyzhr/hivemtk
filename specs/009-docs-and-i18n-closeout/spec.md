# Feature Specification: F1 文档分层 + F3 国际化收口 + C4 多租户文档口径（P2 文档/口径收口）

**Feature Branch**: `009-docs-and-i18n-closeout`

**Created**: 2026-10-24

**Status**: Delivered

**Input**: 对三条 P2 文档/口径收口事项做「规划→设计→调研→论证→开发→测试→交付」：
- **F1**：`docs/INDEX.md`、`hivemtk/docs/handbook/`、`hivemtk/docs/standards/`、`hivemtk/docs/DEV_DOCS_INDEX.md` 已分层（约 50%），补齐分层缺口。
- **F3**：前端 i18n 已 9 语种 + 门禁只许涨；缺口是**后端文案国际化口径与新增页覆盖**。
- **C4**：多租户已实现（`user-server/internal/pkg/tenant/tenant.go`，`TENANT_MODE=off/on`，`specs/001-saas-multi-tenant` 已收口）；只差**文档口径与授权策略复核**。

## 背景与目标

本 lane 不写业务代码（铁律：不触碰 Go 源码、不动 lane 在途文件、不动前端 9 语种 JSON）。
交付形态为**文档/索引/口径**：

1. F1：新增**分层导航页**，把散落文档归入正确层级（优先新增，不移动 lane 在途文件）。
2. F3：产出**后端文案国际化口径与清单**文档；前端 JSON 缺口经取证为 0（门禁已 100% 绿），
   只登记证据、不重复施工。
3. C4：产出**多租户文档口径与授权策略复核**文档；对确有出入的陈旧文档（E4 硬约束、索引头句）做**勘误**。

## 调研取证（本 lane 实测，2026-10-24）

| 取证项 | 结论 | 证据 |
|---|---|---|
| F1 分层缺口 | `hivemtk/docs/` 下 145 篇 `.md`，顶层 `docs/INDEX.md` 未收录 76 篇（覆盖率约 47%）；`docs/architecture/` 无 README、`docs/operations/` 无 README、`docs/standards/` 无 README、`hivemtk/specs/` 无索引 | 全量文件扫描比对 |
| F3 前端 JSON 门禁 | `node user-web/scripts/check-i18n-coverage.cjs` rc=0；zh=1978 key × 9 语种 0 缺失 0 空值；`check-i18n-coverage.sh` rc=0；`$t()` 引用 730 个去重 key，used-but-missing = 0 | 门禁实跑 + 源码扫描 |
| F3 前端硬编码棘轮 | `node user-web/scripts/check-i18n-hardcode.cjs` rc=0（10460 行 / 220 文件，与基线一致）；9 语种 locale JSON 正被并发 lane 修改（working tree 均为 `M`），本 lane 不触碰 | 门禁实跑 + `git status --porcelain` |
| F3 后端文案现状 | 后端已有 `internal/pkg/i18n/`（`ErrorMessages` 错误码四语、`Messages` 语义 key、`BackendPhrases` 中文字面四语词典 100 条、`Localize/T/ErrorMessage`）+ `LocaleMiddleware`（X-Locale > Accept-Language > 中文）；但 `response.*` 调用里含中文字面量的消息共 **184 种**，其中 **127 种未收录** `BackendPhrases`；后端仅 4 语（zh/en/ja/ar），前端 9 语 | 源码实读 + 扫描统计 |
| C4 实现口径 | `tenant.Mode off/on`、`TENANT_MODE` 未知值 fail-safe 朝 off 并报错、`DefaultTenantID=1`、`WithSystemScope` 唯一豁免通道、`Fanout` 后台扇出、`RedisKey` 键分域、`SingleTenantByKey` 行自证、`tenancyidx.Normalize` 唯一索引复合化 | `internal/pkg/tenant/*`、`internal/pkg/tenancyidx/*`、`cmd/api/main.go`、`middleware/tenant.go` |
| C4 文档出入 | ① `DEV_DOCS_INDEX.md` E4「业务表无需多租户字段」已过期；② `hivemtk/docs/INDEX.md` 头句「单商户本地部署（私域模式）」已过期；③ `DATABASE_SCHEMA_DEEP_DIVE.md` §3.1/§X3「单商户 ⇒ 无租户列」与 001 演进反向；④ `research.md` D6 设计名 `TenantRunner/runner.go` 与实现 `Fanout/fanout.go` 命名漂移；⑤ tasks T001 设计名 `SystemTenant/DEFAULT_TENANT_ID` 与实现 `WithSystemScope/DefaultTenantID` 命名漂移 | 文档与代码比对 |

## Requirements

### F1 文档分层

- **FR-001**: 新增 `hivemtk/docs/architecture/README.md`，按「总览 / 深度解析 / 多租户与部署形态 / 模块与专题 / ADR」分层登记 `docs/architecture/` 全部 `.md`。
- **FR-002**: 新增 `hivemtk/docs/operations/README.md` 与 `hivemtk/docs/standards/README.md`，登记各自层级全部文档。
- **FR-003**: 新增 `hivemtk/specs/README.md`，登记全部 spec 施工源（001/002/005/006/009），并注明「spec 是唯一施工源」的既有口径。
- **FR-004**: 在 `hivemtk/docs/INDEX.md` 挂入上述导航页链接；在顶层 `docs/INDEX.md` 的「增量补录」节追加新文档条目（只追加、不重排计数）。

### F3 后端文案国际化收口

- **FR-005**: 新增 `hivemtk/docs/standards/BACKEND_MESSAGE_I18N_POLICY.md`，明确三类口径：
  - **必须 i18n**：API 响应的 `message` 字段（用户可见文案）。
  - **不 i18n**：协议枚举与机器错误码（`ErrorCode` 值本身）、开发日志（`logger.*`）、LLM 生成内容（模型自带多语言）、测试断言与内部调试串。
  - **改造顺序**：错误码注册表 → 语义 key 词典 → 中文字面量词典（`BackendPhrases`/`Localize`）。
- **FR-006**: 该文档须附**清单**：当前后端中文响应消息 184 种、`BackendPhrases` 覆盖 100 种、未覆盖 127 种（分域分组、标注优先级），作为后续 lane 补齐的施工源。
- **FR-007**: 前端 9 语种 JSON 门禁保持 rc=0、key 数不降（本 lane 只取证，不改 JSON）。

### C4 多租户文档口径与授权策略复核

- **FR-008**: 新增 `hivemtk/docs/architecture/TENANT_MODE_REVIEW.md`，逐条核对实现 vs 文档（模式解析、默认租户、隔离 callback、后台扇出、键分域、行自证、授权策略）。
- **FR-009**: 对确认有出入的文档做勘误（新增文档优先）：修正 `hivemtk/docs/INDEX.md` 头句；修正 `DEV_DOCS_INDEX.md` E4 表述并挂入复核文档为权威来源；`DATABASE_SCHEMA_DEEP_DIVE.md` 等大体量文档只在新文档中登记勘误、不整文件改写。
- **FR-010**: 授权策略复核须回答：`WithSystemScope` 谁能调（装配层 / 系统入口 / 平台级扫描）、谁不能调（业务请求路径）；`WithTenantID` 的 id=0 语义；fail-closed 的三条判据。

## Success Criteria

- **SC-001**: 新增导航页 5 篇（architecture / operations / standards / specs / i18n 口径），全部被 `hivemtk/docs/INDEX.md` 或顶层 `docs/INDEX.md` 引用。
- **SC-002**: `check-i18n-coverage.cjs` 与 `check-i18n-hardcode.cjs` 均 rc=0；前端 key 数 1978 不降。
- **SC-003**: C4 复核文档列出全部出入项，并对可低成本修正的陈旧文档完成勘误。
- **SC-004**: `specs/009-docs-and-i18n-closeout/` 四件套齐备，delivery.md 写清改动文件、检查命令与结果、遗留项。

## Assumptions

- `hivemtk/docs/INDEX.md`、`hivemtk/docs/DEV_DOCS_INDEX.md` 经 `git status` 确认干净，可做增量编辑；`docs/architecture/` 下多篇文档可能为 lane 在途，一律不整文件改写，只新增。
- 前端 9 语种 locale JSON 均处于并发 lane 的 working tree 修改中（`M`），本 lane 不触碰。
- 本 lane 不跑 Go 测试、不跑前端构建（node_modules 风险由主人判断）；只跑 i18n 门禁自检（Node/Python 均已在前面实跑）。
