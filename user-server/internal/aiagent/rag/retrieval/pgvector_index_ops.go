package ragretrieval

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/utils/logger"
)

const (
	// HNSWMinRowsForIndex 建议建 HNSW 索引的最小行数（小表建索引得不偿失）
	HNSWMinRowsForIndex int64 = 50000

	// HNSWM HNSW 图的最大连接数
	HNSWM = 16

	// HNSWEfConstruction 建索引时的候选队列长度
	HNSWEfConstruction = 200

	// HNSWEfSearch 查询时候选队列长度
	HNSWEfSearch = 100
)

// EnsureHNSWIndexIfNeeded 迁移辅助：行数超过阈值且索引缺失时创建 HNSW 索引
// （vector_cosine_ops）。created=true 表示本次实际创建了索引；
// 行数未达阈值或索引已存在时不做任何事。
// 注意：本函数使用普通 CREATE INDEX（迁移框架内执行）；生产大表建议运维手工
// 用 CREATE INDEX CONCURRENTLY 执行同等 DDL 以避免锁写。
func EnsureHNSWIndexIfNeeded(ctx context.Context, db *gorm.DB, table, column, indexName string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("db 未初始化")
	}
	table = sanitizeIdent(table)
	column = sanitizeIdent(column)

	var cnt int64
	if err := db.WithContext(ctx).
		Raw(fmt.Sprintf(`SELECT count(*) FROM %s`, table)).
		Scan(&cnt).Error; err != nil {
		return false, fmt.Errorf("统计行数失败: %w", err)
	}
	if cnt < HNSWMinRowsForIndex {
		logger.Infof("[R-7] %s rows=%d < %d, skip hnsw index", table, cnt, HNSWMinRowsForIndex)
		return false, nil
	}

	var exists bool
	if err := db.WithContext(ctx).Raw(`
		SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?)
	`, indexName).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("检查索引存在性失败: %w", err)
	}
	if exists {
		return false, nil
	}

	ddl := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw ((%s) vector_cosine_ops) WITH (m = %d, ef_construction = %d)`,
		sanitizeIdent(indexName), table, column, HNSWM, HNSWEfConstruction,
	)
	if err := db.WithContext(ctx).Exec(ddl).Error; err != nil {
		return false, fmt.Errorf("创建 HNSW 索引失败: %w", err)
	}
	logger.Infof("[R-7] created hnsw index %s on %s(%s), rows=%d", indexName, table, column, cnt)
	return true, nil
}

// ApplyHNSWSessionParams 会话级查询调参（ef_search=100 + iterative_scan）。
// 在执行向量检索的连接/事务内调用；iterative_scan 用于带过滤条件的近邻查询，
// 避免 HNSW 在过滤后候选不足时召回塌陷。pgvector 版本不支持时静默忽略。
func ApplyHNSWSessionParams(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db 未初始化")
	}
	if err := db.WithContext(ctx).Exec(fmt.Sprintf(`SET hnsw.ef_search = %d`, HNSWEfSearch)).Error; err != nil {
		return err
	}

	_ = db.WithContext(ctx).Exec(`SET hnsw.iterative_scan = relaxed_order`).Error
	return nil
}

func sanitizeIdent(s string) string {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_."
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(allowed, r) {
			return r
		}
		return -1
	}, s)
	if clean == "" || clean != s {
		panic(fmt.Sprintf("非法标识符: %q", s))
	}
	return clean
}
