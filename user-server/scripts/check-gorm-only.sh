#!/usr/bin/env bash
# GORM-only 门禁（specs/001-saas-multi-tenant research.md D2）
#
# 业务代码库的数据访问必须统一走 GORM API，禁止 .Exec( / .Raw( 直连式历史债务：
#   - 豁免目录：internal/migration/（DDL 是迁移的合法形态）
#   - 豁免文件：internal/pkg/db/migrate.go（schema 引导 DDL：CREATE EXTENSION/ALTER/DROP INDEX，
#     运行于 AutoMigrate 生命周期，与 migration 同类）
#   - 豁免文件：internal/pkg/tenancyidx/tenancyidx.go（schema 自省/索引规范化 DDL，
#     运行于 AutoMigrate 与迁移生命周期，SystemTenant 路径，与 migration 同类）
#   - 豁免目录：internal/pkg/testutil/（测试库基建 DDL，如 CREATE DATABASE，等价 migration 语义）
#   - 豁免文件：internal/aiagent/rag/retrieval/pgvector_session.go（pgvector 会话 GUC/索引 DDL，
#     GORM 无方言表达，集中单点管理）
#   - 豁免文件：internal/aiagent/rag/retrieval/pgvector_index_ops.go（HNSW 索引 DDL + GUC 调优，
#     同上，整文件即 pgvector 运维域）
#   - 豁免文件：*_test.go（测试夹具，不承载生产隔离语义）
#
# 用法：
#   bash user-server/scripts/check-gorm-only.sh                # strict：违规即 exit 1（门禁）
#   bash user-server/scripts/check-gorm-only.sh --report-only  # 观测：打印违规清单，恒 exit 0
#
# T012-T014 清偿期间 Makefile audit 用 --report-only；T015 归零后切回 strict。
set -euo pipefail

cd "$(dirname "$0")/.."   # → user-server/

MODE="strict"
if [[ "${1:-}" == "--report-only" ]]; then
  MODE="report"
fi

violations=$(grep -rn --include='*.go' -e '\.Exec(' -e '\.Raw(' internal/ 2>/dev/null \
  | grep -v '\.Raw()\.\|\.Row()\.' \
  | grep -v '^internal/migration/' \
  | grep -v '^internal/pkg/testutil/' \
  | grep -v '^internal/pkg/db/migrate\.go:' \
  | grep -v '^internal/pkg/tenancyidx/' \
  | grep -v '^internal/aiagent/rag/retrieval/pgvector_session.go:' \
  | grep -v '^internal/aiagent/rag/retrieval/pgvector_index_ops.go:' \
  | grep -v '_test\.go' \
  || true)

count=$(printf '%s' "$violations" | grep -c . || true)

if [[ "$count" -eq 0 ]]; then
  echo "[gorm-only] ✓ 业务包 0 处 .Exec( / .Raw(（migration DDL 与 *_test.go 已豁免）"
  exit 0
fi

echo "[gorm-only] ✗ 业务包存在 ${count} 处 .Exec( / .Raw( 直连式数据访问（应改为 GORM API）："
printf '%s\n' "$violations" | sed 's/^/    /'

if [[ "$MODE" == "strict" ]]; then
  echo "[gorm-only] 门禁失败：请改为 GORM 链式 API（clause.OnConflict / clause.Locking / clause.Returning / Where 片段参数绑定）。"
  echo "[gorm-only] 改造依据：specs/001-saas-multi-tenant/research.md D2；GORM 表达对照见 specs/001-saas-multi-tenant/tasks.md T012。"
  exit 1
fi

echo "[gorm-only] （report-only 观测模式：债务清偿 T012-T014 进行中，T015 归零后切 strict 门禁）"
exit 0
