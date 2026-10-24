package migrations

import (
	"context"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// ADomainP1Migration A 域 缺口修复迁移（no-op）
type ADomainP1Migration struct {
	db *gorm.DB
}

func NewADomainP1Migration(db *gorm.DB) *ADomainP1Migration {
	return &ADomainP1Migration{db: db}
}

func (m *ADomainP1Migration) Version() string { return "v2.11.0" }

func (m *ADomainP1Migration) Name() string {
	return "A 域 P1 缺口修复 - 团队用户表已并入 system_users"
}

func (m *ADomainP1Migration) Description() string {
	return "阶段 1 单表化重构：team_users 表已 DROP，data_scope 字段保留在 system_users"
}

// Up 执行升级（no-op，team_users 已 DROP）
func (m *ADomainP1Migration) Up(ctx context.Context) error {
	return nil
}

func (m *ADomainP1Migration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*ADomainP1Migration)(nil)
