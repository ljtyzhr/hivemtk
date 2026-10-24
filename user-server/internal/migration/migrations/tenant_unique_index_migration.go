package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/tenancyidx"

	"gorm.io/gorm"
)

// TenantUniqueIndexMigration 业务表唯一索引租户复合化
//
// 原理：行级隔离下，业务唯一性必须升级为租户维度唯一。
// 实现委托框架层规范化器 tenancyidx.Normalize（与 AutoMigrate 路径共用唯一实现）：
// 动态枚举"已含 tenant_id 列的表"上的 CREATE UNIQUE INDEX（排除主键 _pkey 与
// 约束型 UNIQUE——后者可能被 ON CONFLICT 引用），
// 原位重建（沿用原索引名）为 (tenant_id, 原列...)。
//
// 幂等：indexdef 已含 tenant_id 即跳过。单索引独立事务，失败即中止（fail-closed）。
type TenantUniqueIndexMigration struct {
	db *gorm.DB
}

func NewTenantUniqueIndexMigration(db *gorm.DB) *TenantUniqueIndexMigration {
	return &TenantUniqueIndexMigration{db: db}
}

func (m *TenantUniqueIndexMigration) Version() string { return "v3.59.0" }

func (m *TenantUniqueIndexMigration) Name() string { return "业务表唯一索引租户复合化" }

func (m *TenantUniqueIndexMigration) Description() string {
	return "SaaS 多租户：已租户化表上的非约束型唯一索引原位重建为 (tenant_id, 原列...)，业务唯一性升级为租户维度；幂等可重入"
}

// Up 执行升级：委托框架层规范化器（唯一实现，AutoMigrate 路径共用）
func (m *TenantUniqueIndexMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	n, err := tenancyidx.Normalize(m.db.WithContext(ctx))
	if err != nil {
		return err
	}
	if n > 0 {
		fmt.Printf("[v3.59.0] 唯一索引复合化完成：处理 %d 个索引\n", n)
	}
	return nil
}

// Down 不回滚：唯一性升级是数据正确性约束，回滚会重新打开跨租户冲突面
func (m *TenantUniqueIndexMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*TenantUniqueIndexMigration)(nil)
