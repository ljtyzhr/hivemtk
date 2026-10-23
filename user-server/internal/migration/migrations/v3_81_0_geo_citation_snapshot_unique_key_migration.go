package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// GeoCitationSnapshotUniqueKeyMigration v3.81.0（A10 引用漂移，2026-10-24）：
// 为 geo_citation_snapshots 建立 (tenant_id, snapshot_date) 唯一索引。
//
// 与 v3.71.0（geo_daily_stats）同一取舍：模型**有意**不用 uniqueIndex 标签
// （见 model/citation_snapshot.go 头注——AutoMigrate 多模型同键先到先得会静默
// 复用错形状），唯一性由本迁移建。快照写入走 ON CONFLICT (tenant_id, snapshot_date)
// UPSERT，全新库上没有这条索引就是 42P10，快照任务整条写不进去。
// 全新部署 AutoMigrate 建表后本迁移建索引；幂等可重入。
type GeoCitationSnapshotUniqueKeyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*GeoCitationSnapshotUniqueKeyMigration)(nil)

func NewGeoCitationSnapshotUniqueKeyMigration(db *gorm.DB) *GeoCitationSnapshotUniqueKeyMigration {
	return &GeoCitationSnapshotUniqueKeyMigration{db: db}
}

func (m *GeoCitationSnapshotUniqueKeyMigration) Version() string { return "v3.81.0" }

func (m *GeoCitationSnapshotUniqueKeyMigration) Name() string {
	return "geo_citation_snapshots 唯一键"
}

func (m *GeoCitationSnapshotUniqueKeyMigration) Description() string {
	return "建立 (tenant_id, snapshot_date) 唯一索引，支撑快照 UPSERT 的 ON CONFLICT；幂等可重入"
}

func (m *GeoCitationSnapshotUniqueKeyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("geo_citation_snapshots") {
		return nil
	}
	// 去重再建（同日多行时保留 id 最大的——它的聚合覆盖面最全）
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			DELETE FROM geo_citation_snapshots a
			USING geo_citation_snapshots b
			WHERE a.tenant_id = b.tenant_id
			  AND a.snapshot_date = b.snapshot_date
			  AND a.id < b.id`).Error; err != nil {
			return fmt.Errorf("同日重复快照去重失败: %w", err)
		}
		if err := tx.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_geo_citation_snapshots_tenant_date
			ON geo_citation_snapshots (tenant_id, snapshot_date)`).Error; err != nil {
			return fmt.Errorf("建唯一索引失败: %w", err)
		}
		return nil
	})
}

func (m *GeoCitationSnapshotUniqueKeyMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	return m.db.WithContext(ctx).Exec(
		`DROP INDEX IF EXISTS idx_geo_citation_snapshots_tenant_date`).Error
}
