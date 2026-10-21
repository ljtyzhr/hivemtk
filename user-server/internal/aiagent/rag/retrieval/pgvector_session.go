// Package retrieval 的 pgvector 会话/索引操作集中点（specs/001-saas-multi-tenant T013）。
//
// 本文件豁免 GORM-only 硬闸（check-gorm-only.sh）：SET LOCAL（会话 GUC）与
// pgvector 索引 DDL（CREATE INDEX ... USING hnsw）是数据库方言操作，
// GORM 无对应表达；集中于此单点管理，数据访问本身仍一律走 GORM API。
package ragretrieval

import (
	"fmt"

	"gorm.io/gorm"
)

// SetLocalHnswEfSearch 事务内设置 hnsw.ef_search（SET LOCAL 随事务结束自动还原）
func SetLocalHnswEfSearch(tx *gorm.DB, efSearch int) error {
	if err := tx.Exec(fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", efSearch)).Error; err != nil {
		return fmt.Errorf("set hnsw.ef_search: %w", err)
	}
	return nil
}

// EnsureVectorIndexOps pgvector 索引/扩展运维入口（建索引、GUC 调优等）。
// 具体 DDL 详见 pgvector_index_ops.go（同样豁免）。
func EnsureVectorIndexOps(db *gorm.DB) *gorm.DB { return db }
