package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// CustomerSessionUpdatedAtMigration customer_sessions 补齐 updated_at 列迁移
type CustomerSessionUpdatedAtMigration struct {
	db *gorm.DB
}

func NewCustomerSessionUpdatedAtMigration(db *gorm.DB) *CustomerSessionUpdatedAtMigration {
	return &CustomerSessionUpdatedAtMigration{db: db}
}

func (m *CustomerSessionUpdatedAtMigration) Version() string { return "v2.12.0" }

func (m *CustomerSessionUpdatedAtMigration) Name() string {
	return "customer_sessions 补齐 updated_at 列"
}

func (m *CustomerSessionUpdatedAtMigration) Description() string {
	return "补齐 customer_sessions 缺失的 updated_at 列，修复全渠道会话 upsert 的 SQLSTATE 42703"
}

func (m *CustomerSessionUpdatedAtMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if !m.db.Migrator().HasTable("customer_sessions") {
		return nil
	}

	stmts := []string{
		`ALTER TABLE customer_sessions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`,
	}
	if err := execAllCS(ctx, m.db, stmts); err != nil {
		return fmt.Errorf("补齐 customer_sessions.updated_at 失败: %w", err)
	}
	return nil
}

// Down 执行降级（不删除列，避免误删数据；如需回滚请手动处理）
func (m *CustomerSessionUpdatedAtMigration) Down(ctx context.Context) error {
	return nil
}

func execAllCS(ctx context.Context, db *gorm.DB, stmts []string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	for _, sql := range stmts {
		if err := db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("exec failed (%s): %w", sql, err)
		}
	}
	return nil
}

var _ migration.Migration = (*CustomerSessionUpdatedAtMigration)(nil)
