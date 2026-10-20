# Fix Report: 迁移 Down 不再销毁 AutoMigrate 已持有的 6 张表

- **Slug**: migration-down-drops-automigrate-owned-tables
- **Fixed**: 2026-10-20
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

`internal/migration/migrations` 的 4 个迁移在 **Down** 分支里 `DROP TABLE` 掉了 6 张表。这 6 张表在本轮在途改动后已被登记为 GORM 模型（`internal/pkg/db/migrate.go` 的 `allModels()`），**建表事实源是 AutoMigrate**，对应迁移的 Up 里的 `CREATE TABLE IF NOT EXISTS` 实际是空操作 —— 表不是这些迁移建的，Down 却没有表可删，删掉的其实是 AutoMigrate 刚建的表：

- 一轮 Up→Down 后基线表净丢失 6 张
- 连带该 6 张表的索引、列一起消失（测试断言「索引 4/97 丢失」「列 4/97 丢失」）
- 降级路径上术语表 / 知识库 / 绑定关系 / RFM 分层 / 邮件指标数据不可恢复地丢失

修法沿用仓库既有降级放弃出口 `declineTableDrop`（`down_drop_policy.go:21-31`）：**声明放弃 + 从 stmts 里去掉 DROP 语句**。不改 `allModels()`、不改公共 drop 策略、不改测试断言。

## Changes

| 文件 | 迁移 | 改动 |
|------|------|------|
| `user-server/internal/migration/migrations/compliance_d_e_migration.go` | v2.10.1 | `declineTableDrop` 清单加 `email_job_metrics`、`email_tracking_events`；stmts 只保留 `DROP TABLE IF EXISTS sms_job_metrics` |
| `user-server/internal/migration/migrations/multilingual_i18n_migration.go` | v3.12.0 | 新增 `declineTableDrop(m.Version(), "glossaries")`；stmts 改为空 |
| `user-server/internal/migration/migrations/v3_15_kb_unification_migration.go` | v3.15.0 | 新增 `declineTableDrop(m.Version(), "agent_kb_bindings", "knowledge_bases")`；stmts 改为空 |
| `user-server/internal/migration/migrations/h_p1_migration.go` | v3.2.0 | `declineTableDrop` 清单加 `customer_rfm`；stmts 改为空 |

每处都带中文注释说明：AutoMigrate 是建表事实源 ⇒ 迁移的 Up 里 CREATE 是空操作 ⇒ 这张表不是本迁移建的，Down 不得销毁（数据丢失不可恢复）。

## Diff Highlights

`user-server/internal/migration/migrations/h_p1_migration.go`：

```diff
-	declineTableDrop(m.Version(), "recovery_queue", "clue_engagement_events", "clue_scores")
-	stmts := []string{
-		`DROP TABLE IF EXISTS customer_rfm`,
-	}
+	// customer_rfm 已登记为 GORM 模型（&model.CustomerRFM{}，见 internal/pkg/db/migrate.go 的 allModels），
+	// 建表事实源是 AutoMigrate ⇒ 本迁移的 Up 里的 CREATE TABLE IF NOT EXISTS 是空操作，
+	// 这张表不是本迁移建的，Down 不得销毁它（RFM 分层数据丢失不可恢复）。
+	declineTableDrop(m.Version(), "recovery_queue", "clue_engagement_events", "clue_scores", "customer_rfm")
+	stmts := []string{}
```

`user-server/internal/migration/migrations/compliance_d_e_migration.go`：

```diff
 	declineTableDrop(m.Version(), "sms_delivery_statuses", "sms_unsubscribes", "email_unsubscribes",
+		"email_job_metrics", "email_tracking_events")
 	stmts := []string{
 		`DROP TABLE IF EXISTS sms_job_metrics`,
-		`DROP TABLE IF EXISTS email_job_metrics`,
-		`DROP TABLE IF EXISTS email_tracking_events`,
 	}
```

## Tests Added or Updated

- 未新增测试文件、未改测试断言：既有 `TestFullMigrationChainUpThenRollback`（`user-server/internal/migration/migrations/a_full_chain_migration_test.go`）已精确覆盖本缺陷的判定条件（越界删表 0/97、索引 0/97、列 0/97、基线表净丢失 0 张）。

## Local Verification

```bash
cd user-server
PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/migration/migrations -count=1 -timeout 900s
```

结果：`ok hivemtk-user/internal/migration/migrations 26.342s`（RC=0），修复前为 `FAIL TestFullMigrationChainUpThenRollback 16.14s`。

`gofmt -l` 干净、`go build ./...` 通过。

## Deviations from Assessment

- 无。assessment 的「Files likely to change」正是这 4 个文件，只改 Down 分支；`allModels()`、公共 drop 策略、测试断言均未动。

## Follow-ups

- 无。既有 1 处 Down 失败（v3.28.0 email_smtp「不支持回滚」）与本缺陷无关，属预期内，不在本 slug 范围。
- `sms_job_metrics` 仍由 v2.10.1 Down 删除：它未登记为模型，属该迁移自建对象，删除合法。
