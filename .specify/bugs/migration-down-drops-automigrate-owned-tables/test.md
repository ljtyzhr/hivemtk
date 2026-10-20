# Bug Test: 迁移 Down 不再销毁 AutoMigrate 已持有表的回归验证

- **Slug**: migration-down-drops-automigrate-owned-tables
- **Tested**: 2026-10-20
- **Fix**: [fix.md](fix.md)
- **Result**: verified
- **Status**: verified

## Verification Commands

### 1. 缺陷包回归（核心判定门）

```bash
cd user-server
PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/migration/migrations -count=1 -timeout 900s
```

**修复前**：`FAIL hivemtk-user/internal/migration/migrations 16.14s`
（`--- FAIL: TestFullMigrationChainUpThenRollback`：Down 删掉了不属于它的表 4/97、索引 4/97、列 4/97、一轮 Up→Down 后基线表净丢失 6 张）

**修复后**：`ok hivemtk-user/internal/migration/migrations 26.342s`（RC=0）✅

### 2. 静态确认：4 处放弃声明 + 6 条 DROP 已移除

```bash
cd user-server/internal/migration/migrations
grep -n "declineTableDrop\|stmts :=\|DROP TABLE" \
  compliance_d_e_migration.go multilingual_i18n_migration.go \
  v3_15_kb_unification_migration.go h_p1_migration.go
```

**结果**：

| 文件 | 行 | 放弃声明 | stmts 剩余 DROP |
|------|----|---------|----------------|
| `compliance_d_e_migration.go` | 219 / 221 | `sms_delivery_statuses, sms_unsubscribes, email_unsubscribes, email_job_metrics, email_tracking_events` | 仅 `sms_job_metrics`（无模型持有，合法删除） |
| `multilingual_i18n_migration.go` | 94 / 96 | `glossaries` | 无（`stmts := []string{}`） |
| `v3_15_kb_unification_migration.go` | 181 / 183 | `agent_kb_bindings, knowledge_bases` | 无（`stmts := []string{}`） |
| `h_p1_migration.go` | 171 / 172 | `recovery_queue, clue_engagement_events, clue_scores, customer_rfm` | 无（`stmts := []string{}`） |

### 3. 编译与格式

```bash
cd user-server && go build ./... && gofmt -l internal/migration/migrations
```

**结果**：`BUILD_OK`；`gofmt -l` 输出为空 ✅

## Test Results

| 检查项 | 结果 |
|--------|------|
| `internal/migration/migrations` 包回归（`TestFullMigrationChainUpThenRollback`） | ✅ 修复后 `ok 26.342s` |
| Down 越界删表 0/97、索引 0/97、列 0/97、基线表净丢失 0 张 | ✅ 用例断言通过 |
| 4 处 `declineTableDrop` 声明就位 | ✅ grep 核对 |
| 6 条 `DROP TABLE` 已从 stmts 移除 | ✅ grep 核对 |
| `sms_job_metrics` 仍可删（无模型持有） | ✅ 保留 DROP |
| `go build ./...` + `gofmt -l` | ✅ 干净 |

## Conclusion

迁移 Down 不再销毁 AutoMigrate 已持有的 6 张表（`email_job_metrics`、`email_tracking_events`、`glossaries`、`agent_kb_bindings`、`knowledge_bases`、`customer_rfm`）。一轮全链 Up→Down 后基线表不再净丢失，降级路径上的术语表 / 知识库 / 绑定关系 / RFM 分层 / 邮件指标数据保住。修复方式与仓库既有降级放弃出口（`down_drop_policy.go`）一致，未改公共策略、未改测试断言。
