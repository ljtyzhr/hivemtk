# Bug Assessment: 迁移 Down 误删 AutoMigrate 已持有的表，导致全链 Up→Down 基线表净丢失

- **Slug**: migration-down-drops-automigrate-owned-tables
- **Created**: 2026-10-20
- **Source**: 自查（`go test ./internal/migration/migrations` 回归，`TestFullMigrationChainUpThenRollback` 失败）
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

用户指令（2026-10-20）：「前置任务一：3 对抗性 检查、审核、测试 整个 saas多租户改造 发现问题立即修复」。本轮在途改动把 8 个模型登记进 AutoMigrate 基线后，全量测试暴露迁移链回归。

## Symptom

`go test ./internal/migration/migrations` 失败：`TestFullMigrationChainUpThenRollback`。

关键断言（`user-server/internal/migration/migrations/a_full_chain_migration_test.go`）：

- `:189` `Down 删掉了不属于它的表 4/97` ⇒ 命中 4 个版本：
  - v2.10.1 `email_job_metrics` / `email_tracking_events`
  - v3.12.0 `glossaries`
  - v3.15.0 `agent_kb_bindings` / `knowledge_bases`
  - v3.2.0 `customer_rfm`
- `:193` 索引 4/97、`:197` 列 4/97 ⇒ 同源（表被删 ⇒ 其索引/列一并消失）
- `:210` 一轮 Up→Down 后基线表净丢失 6 张
- `:186` `Down 失败 1/97：v3.28.0/email_smtp … 不支持回滚` ⇒ 既有、预期内、不可回滚，与本缺陷无关
- `:63` 基线铺设完成 表数=346 索引数=1246 列数=4700；`:234` 迁移总数=97

HEAD 基线（未改代码）该包 `ok 28.867s` ⇒ **本缺陷由本轮在途改动引入**，非既有债。

## Reproduction

```bash
cd user-server
PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/migration/migrations -count=1 -p 1 -timeout 900s
```

输出：`--- FAIL: TestFullMigrationChainUpThenRollback (16.14s)`，并打印上述 4/97 越界删表、4/97 索引、4/97 列、净丢 6 张基线表。

## Suspected Code Paths

- `user-server/internal/pkg/db/migrate.go` — `allModels()`：本轮在途改动新登记 8 个模型
  （`&model.EmailTrackingEvent{}`、`&model.EmailJobMetric{}`、`&model.WechatAccount{}`、
  `&model.WechatMessage{}`、`&model.CustomerRFM{}`、`&model.KnowledgeBase{}`、
  `&model.AgentKBBinding{}`、`&model.Glossary{}`）⇒ AutoMigrate 基线现在含这 6 张表
  （`wechat_accounts` / `wechat_messages` 未被任何迁移 Down 删除，故不在越界清单）。
- `user-server/internal/migration/migrations/compliance_d_e_migration.go` — v2.10.1 Down：
  `DROP TABLE IF EXISTS sms_job_metrics / email_job_metrics / email_tracking_events`
- `user-server/internal/migration/migrations/multilingual_i18n_migration.go` — v3.12.0 Down：
  `DROP TABLE IF EXISTS glossaries`
- `user-server/internal/migration/migrations/v3_15_kb_unification_migration.go` — v3.15.0 Down：
  `DROP TABLE IF EXISTS agent_kb_bindings / knowledge_bases`
- `user-server/internal/migration/migrations/h_p1_migration.go` — v3.2.0 Down：
  `DROP TABLE IF EXISTS customer_rfm`
- `user-server/internal/migration/migrations/down_drop_policy.go:21-31` — 统一降级放弃出口
  （`declineDrop` / `declineTableDrop` / `declineColumnDrop` / `declineIndexDrop`）

## Root Cause Hypothesis

**根因（high）**：本仓的建表事实源是 GORM AutoMigrate，版本化迁移的 Down 只能撤销「自己 Up 新建的」对象；
而这 4 个版本的 Down 仍在 `DROP TABLE` 那些后来被登记为 GORM 模型的表。模型一旦登记进 `allModels()`，
AutoMigrate 就会在基线里建好这些表，Down 再删就变成「删掉了不属于它的表」——生产降级会把正在使用的
业务表（邮件指标、邮件追踪、术语表、知识库绑定、客户 RFM）直接销毁，数据不可恢复。

**缺的那道闸（high）**：这 4 个版本的 Down 没有像全仓 20+ 处迁移那样调用既有的
`declineTableDrop(...)` 放弃出口，缺了「这张表由模型/AutoMigrate 持有 ⇒ 不销毁」的保护。

## Proposed Remediation

### Preferred

沿用仓库既有降级放弃出口，给这 4 个版本的 Down 补 `declineTableDrop(...)`：

| 版本 | 文件 | 需放弃删除的表 | 备注 |
|------|------|----------------|------|
| v2.10.1 | `compliance_d_e_migration.go` | `email_job_metrics`、`email_tracking_events` | `sms_job_metrics` 未登记为模型，仍由本迁移拥有，继续删除 |
| v3.12.0 | `multilingual_i18n_migration.go` | `glossaries` | 已有 declineIndexDrop/declineColumnDrop，只缺表级 |
| v3.15.0 | `v3_15_kb_unification_migration.go` | `agent_kb_bindings`、`knowledge_bases` | 同上 |
| v3.2.0 | `h_p1_migration.go` | `customer_rfm` | 已有 declineTableDrop 清单，只缺这一张 |

放弃删表 ⇒ 这 4 张表的列与索引也一并保留 ⇒ `:193` `:197` 的列/索引净丢失同时消失。
不引入新机制、不改 97 个迁移的通用语义，改动集中在 4 个文件的 Down 分支。

### Alternatives (optional)

- 动态判定：`user-server/internal/model/module_tables.go:256` 的 `model.OwnerOfTable(table)` 可判断表是否由模型持有
  （`TestEveryModelTableHasModuleOwner` 钉死覆盖全部模型表），据此自动放弃删除。**未采纳**：会改变全部 97 个迁移的
  Down 语义，侵入面与回归风险远超本轮范围。
- 回退 `allModels()` 的 8 个模型登记：**未采纳**：那 8 个模型有生产写路径（邮件指标、企微账号、RFM、知识库、术语表），
  缺了 AutoMigrate 就没有表，属于把生产可用性换测试好看。

### Files likely to change

- `user-server/internal/migration/migrations/compliance_d_e_migration.go`
- `user-server/internal/migration/migrations/multilingual_i18n_migration.go`
- `user-server/internal/migration/migrations/v3_15_kb_unification_migration.go`
- `user-server/internal/migration/migrations/h_p1_migration.go`

（仅改这 4 个文件的 Down 分支；不改 `allModels()`、不改公共 drop 策略、不改测试断言。）

### Tests to add or update

- 复用既有 `TestFullMigrationChainUpThenRollback`（`a_full_chain_migration_test.go`）作为回归门：
  要求 `Down 删掉了不属于它的表 0/97`、索引 0/97、列 0/97、基线表净丢失 0 张。
- 不新增测试文件：该用例已精确覆盖本缺陷的判定条件（越界删表 / 净丢失）。

## Risks & Considerations

- 放弃删表后，Down 不再销毁这 6 张表 ⇒ 迁移降级后它们留在库里（表由 AutoMigrate 持有，语义正确）。
- `sms_job_metrics` 仍被 v2.10.1 Down 删除：它未登记为模型，属该迁移自建对象，删除合法。
- 既有 1 处 Down 失败（v3.28.0 email_smtp 不支持回滚）与本缺陷无关，不在范围内。

## Open Questions

- 无。修复口径与仓库既有 `declineTableDrop` 用法一致，无需额外澄清。
