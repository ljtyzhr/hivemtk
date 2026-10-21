package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// TenantSubscriptionMirrorMigration 订阅镜像表（003 T022 · ADR-M8 修订 #4）
//
// 建表时是部署级单行；003 生产联调后改为**每租户一行**（行锚 id = tenant_id，
// tenant_id 列由启动期 AutoMigrate 按模型补齐）。后台同步跑在 SystemScope，
// 落默认租户那一行；租户内手动同步保留租户绑定，落本租户。
//
//	tenant_subscription_mirror
//	  id                  PK（行锚 = tenant_id；off 模式恒 1）
//	  tenant_id           BIGINT（隔离列，模型声明，AutoMigrate 补）
//	  status              VARCHAR(16)  —— 判定缓存列，权威 = CurrentStatus()
//	  current_period_end  TIMESTAMPTZ NULL
//	  trial_expires_at    TIMESTAMPTZ NULL
//	  price_cents         BIGINT       —— 月价快照（分）
//	  fetched_at          TIMESTAMPTZ  —— 只 Warn 不参与锁（修订 #2）
type TenantSubscriptionMirrorMigration struct {
	db *gorm.DB
}

// NewTenantSubscriptionMirrorMigration 创建迁移实例
func NewTenantSubscriptionMirrorMigration(db *gorm.DB) *TenantSubscriptionMirrorMigration {
	return &TenantSubscriptionMirrorMigration{db: db}
}

func (m *TenantSubscriptionMirrorMigration) Version() string { return "v3.64.0" }

func (m *TenantSubscriptionMirrorMigration) Name() string {
	return "tenant_subscription_mirror"
}

func (m *TenantSubscriptionMirrorMigration) Description() string {
	return "v3.64.0：新增 tenant_subscription_mirror 订阅镜像表（003 T022；后经 003 联调改为每租户一行）；幂等可重入"
}

func (m *TenantSubscriptionMirrorMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	ddl := `
CREATE TABLE IF NOT EXISTS tenant_subscription_mirror (
	id SMALLINT PRIMARY KEY DEFAULT 1,
	status VARCHAR(16) NOT NULL DEFAULT '',
	current_period_end TIMESTAMPTZ NULL,
	trial_expires_at TIMESTAMPTZ NULL,
	price_cents BIGINT NOT NULL DEFAULT 0,
	fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`
	if err := db.Exec(ddl).Error; err != nil {
		return fmt.Errorf("创建 tenant_subscription_mirror 失败: %w", err)
	}
	return nil
}

// Down 不回滚：镜像行是判定输入，删表会让订阅闸退化 fail-open（锁不住）
func (m *TenantSubscriptionMirrorMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*TenantSubscriptionMirrorMigration)(nil)
