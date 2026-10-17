package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// TenantGlobalEmailUniqueMigration 登录邮箱跨租户全局唯一索引（specs/001-saas-multi-tenant T029/T031）
//
// 背景：data-model.md 规定「邮箱全局唯一登录通道」——跨租户歧义的用户名由邮箱消解，
// 因此 email 的唯一性语义是**全局**，与 username 的「租户内唯一」相反。
//
// 为什么需要数据库兜底：开户事务内的应用层查重在读已提交隔离下留有竞态窗口
// （两个并发开户请求可同时通过 EmailExists 检查）。若不加索引，跨租户重复邮箱
// 会落库，导致登录通道解析出多个租户。此处用部分唯一索引 fail-closed 兜住。
//
// 命名约定 global_uniq_ 前缀：tenancyidx 的唯一索引复合化按此前缀豁免
// （复合化会把 email 索引改成 (tenant_id, email)，恰好摧毁全局唯一语义）。
//
// 幂等：IF NOT EXISTS + 前置重复值体检；存量重复邮箱 fail-closed 并报出冲突值。
type TenantGlobalEmailUniqueMigration struct {
	db *gorm.DB
}

// NewTenantGlobalEmailUniqueMigration 创建邮箱全局唯一索引迁移
func NewTenantGlobalEmailUniqueMigration(db *gorm.DB) *TenantGlobalEmailUniqueMigration {
	return &TenantGlobalEmailUniqueMigration{db: db}
}

// Version 返回版本号
//
// v3.61.0 而非 v3.60.0：v3.60.0 已被 AggregationWatermarkPKMigration 占用，
// 而迁移注册表对重复版本是**启动即 panic**（duplicate migration version）——
// 两个迁移撞号会让整个进程起不来，而症状出现在启动那一刻，与"谁改的"毫无关系。
func (m *TenantGlobalEmailUniqueMigration) Version() string { return "v3.61.0" }

// Name 返回迁移名称
func (m *TenantGlobalEmailUniqueMigration) Name() string {
	return "登录邮箱跨租户全局唯一索引"
}

// Description 返回迁移描述
func (m *TenantGlobalEmailUniqueMigration) Description() string {
	return "SaaS 多租户：system_users.email 建部分唯一索引 global_uniq_system_users_email（lower(email)，排除空值），保证登录邮箱跨租户全局唯一；幂等可重入，存量重复值 fail-closed"
}

// Up 执行升级：重复值体检 → 建部分唯一索引
func (m *TenantGlobalEmailUniqueMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)

	// 前置体检：存量非空邮箱若已有重复，建索引必然失败——先报出冲突值再中止
	var dups []struct {
		Email  string
		Counts int64
	}
	if err := db.Raw(`
		SELECT lower(btrim(email)) AS email, COUNT(*) AS counts
		FROM system_users
		WHERE btrim(coalesce(email, '')) <> ''
		GROUP BY lower(btrim(email))
		HAVING COUNT(*) > 1
		ORDER BY counts DESC, email
		LIMIT 10
	`).Scan(&dups).Error; err != nil {
		return fmt.Errorf("邮箱重复值体检失败: %w", err)
	}
	if len(dups) > 0 {
		msg := "存在重复邮箱，无法建立全局唯一索引（请先清理）: "
		for _, d := range dups {
			msg += fmt.Sprintf("%s×%d ", d.Email, d.Counts)
		}
		return fmt.Errorf("%s", msg)
	}

	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS global_uniq_system_users_email
		ON system_users (lower(btrim(email)))
		WHERE btrim(coalesce(email, '')) <> ''
	`).Error; err != nil {
		return fmt.Errorf("创建邮箱全局唯一索引失败: %w", err)
	}
	return nil
}

// Down 不回滚：删除索引会重新打开跨租户重复邮箱导致登录通道歧义的数据正确性缺口
func (m *TenantGlobalEmailUniqueMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*TenantGlobalEmailUniqueMigration)(nil)
