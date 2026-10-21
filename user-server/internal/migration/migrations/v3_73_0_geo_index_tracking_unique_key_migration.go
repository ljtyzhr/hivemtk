package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// GeoIndexTrackingUniqueKeyMigration v3.73.0：为 geo_index_trackings 建立
// (tenant_id, article_id, engine) 唯一索引。
//
// 事实链：写入路径原为「先 UPDATE，0 行再 INSERT」，并发验证同一文章同一引擎时
// 两个请求都可能 UPDATE 到 0 行后各插一行，漏斗统计与站位评分随重复行失真。
// 仓储已改为 ON CONFLICT DO UPDATE，需要与列集一致的唯一索引。
//
// 存量库可能有重复行，而 AutoMigrate 先于迁移执行且不做去重，因此索引只在
// 本迁移里创建（先按该键保留 id 最大的一行，再建索引）。幂等可重入。
type GeoIndexTrackingUniqueKeyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*GeoIndexTrackingUniqueKeyMigration)(nil)

func NewGeoIndexTrackingUniqueKeyMigration(db *gorm.DB) *GeoIndexTrackingUniqueKeyMigration {
	return &GeoIndexTrackingUniqueKeyMigration{db: db}
}

func (m *GeoIndexTrackingUniqueKeyMigration) Version() string { return "v3.73.0" }

func (m *GeoIndexTrackingUniqueKeyMigration) Name() string {
	return "geo_index_trackings 唯一键租户化"
}

func (m *GeoIndexTrackingUniqueKeyMigration) Description() string {
	return "去重后建立 (tenant_id, article_id, engine) 唯一索引，保证验证结果幂等写入；幂等可重入"
}

func (m *GeoIndexTrackingUniqueKeyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("geo_index_trackings") {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			DELETE FROM geo_index_trackings a
			USING geo_index_trackings b
			WHERE a.id < b.id
			  AND a.tenant_id = b.tenant_id
			  AND a.article_id = b.article_id
			  AND a.engine = b.engine`).Error; err != nil {
			return fmt.Errorf("删除重复收录追踪失败: %w", err)
		}
		if err := tx.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_tracking_article_engine
			ON geo_index_trackings (tenant_id, article_id, engine)`).Error; err != nil {
			return fmt.Errorf("创建收录追踪唯一索引失败: %w", err)
		}
		return nil
	})
}

// Down 不回滚：唯一索引是收录追踪的正确最终形状。
func (m *GeoIndexTrackingUniqueKeyMigration) Down(ctx context.Context) error {
	return nil
}
