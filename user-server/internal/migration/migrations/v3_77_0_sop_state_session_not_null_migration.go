package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// SOPStateMemorySessionIDNotNullMigration v3.77.0（D-6）：sop_state_memories.session_id
// 加 NOT NULL。
//
// 前置事实（规划 §3.6，2026-10-23）：本机开发库 NULL 行 0；模型此前可空。写入通路唯一
// （service.L3SaveSOPState → repository.SaveSOPState）且无生产调用方 ⇒ 存量 NULL 风险极小。
// 但**不在迁移里代拍生产**：Up 之前有预检——若发现 NULL 行，返回明确错误让运维先决定
// 空值行的业务处置（补 session、归档或删除），而不是静默删数据。
//
// 幂等可重入：列已 NOT NULL 时跳过。
type SOPStateMemorySessionIDNotNullMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*SOPStateMemorySessionIDNotNullMigration)(nil)

func NewSOPStateMemorySessionIDNotNullMigration(db *gorm.DB) *SOPStateMemorySessionIDNotNullMigration {
	return &SOPStateMemorySessionIDNotNullMigration{db: db}
}

func (m *SOPStateMemorySessionIDNotNullMigration) Version() string { return "v3.77.0" }

func (m *SOPStateMemorySessionIDNotNullMigration) Name() string {
	return "sop_state_memories.session_id NOT NULL"
}

func (m *SOPStateMemorySessionIDNotNullMigration) Description() string {
	return "session_id 加 NOT NULL（失败前置检查：存在 NULL 行则报错不执行，处置决定留给运维）"
}

func (m *SOPStateMemorySessionIDNotNullMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("sop_state_memories") {
		return nil // 全新部署没有这张表，模型建表时 tag 已带 not null
	}

	// 失败前置检查（规划 §3.6 第 2 条：不能只改 GORM tag 就宣称完成）。
	var nullCount int64
	if err := db.Table("sop_state_memories").
		Where("session_id IS NULL").Count(&nullCount).Error; err != nil {
		return fmt.Errorf("预检 NULL 计数失败: %w", err)
	}
	if nullCount > 0 {
		return fmt.Errorf(
			"sop_state_memories 有 %d 行 session_id 为 NULL：先决定这批行的业务处置（补 session、归档或删除）再加约束，本迁移拒绝静默处理", nullCount)
	}

	// 已是 NOT NULL 则跳过（幂等）。
	nullable, err := columnIsNullable(db, "sop_state_memories", "session_id")
	if err != nil {
		return err
	}
	if !nullable {
		return nil
	}
	return db.Exec(`ALTER TABLE sop_state_memories ALTER COLUMN session_id SET NOT NULL`).Error
}

func (m *SOPStateMemorySessionIDNotNullMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	return m.db.WithContext(ctx).
		Exec(`ALTER TABLE sop_state_memories ALTER COLUMN session_id DROP NOT NULL`).Error
}

// columnIsNullable 查 information_schema（仅本迁移用；PG 方言）。
func columnIsNullable(db *gorm.DB, table, column string) (bool, error) {
	var nullable string
	err := db.Raw(`
		SELECT is_nullable FROM information_schema.columns
		WHERE table_name = ? AND column_name = ? LIMIT 1`, table, column).
		Scan(&nullable).Error
	if err != nil {
		return false, fmt.Errorf("查 information_schema 失败: %w", err)
	}
	if nullable == "" {
		return false, fmt.Errorf("列 %s.%s 不存在", table, column)
	}
	return nullable == "YES", nil
}
