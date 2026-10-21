package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// ChannelBindingChatIDUniqueKeyMigration v3.70.0：channel_agent_bindings 的唯一键
// 从 (tenant_id, channel_type, account_id) 改为
// (tenant_id, channel_type, account_id, COALESCE(chat_id, ”))。
//
// 事实链（三段都验过，
//
//  1. v3.15 迁移（KBUnificationMigration）建的是
//     `idx_channel_binding UNIQUE (channel_type, account_id)`，随后被
//     `tenancyidx.Normalize` 改写成 `(tenant_id, channel_type, account_id)`——
//     **不含 chat_id**。而 chat_id 是模型自己声明的列（"群组/会话 ID，NULL=account 级默认"），
//     telegram 群组功能正是靠它区分「同一账号下的多个群」。
//
//  2. 开发库里的实际索引是另一个形状：
//     `idx_channel_binding_unique UNIQUE (tenant_id, channel_type, account_id,
//     COALESCE(chat_id, ”::character varying))`——含 chat_id，且 chat_id 为 NULL 与空串
//     视为同一个键（account 级默认只允许一条）。
//     但**仓内没有任何迁移建它**：全仓搜 `idx_channel_binding_unique` 只命中
//     backup_20261019 的库快照 ⇒ 它是一次性手工操作留下的，开发库与「按仓内迁移重建的库」
//     已经漂移。
//
//  3. 漂移的后果是生产会坏：开发库 telegram 账号 9 有 3 行（1 条 account 级主绑定
//     + 2 条 chat 级），这三个 (tenant_id, channel_type, account_id) 相同的行在
//     **不含 chat_id 的索引下只剩 1 个键** ⇒ 按仓内迁移重建的库根本建不出这 3 行，
//     telegram 群组绑定直接写不进去。
//
// 本迁移把唯一键改成含 chat_id 的形状，与开发库、与 telegram 群组功能的需求一致。
//
// 为什么用 COALESCE(chat_id, ”) 而不是直接用 chat_id：PG 的唯一索引里 NULL 互不相等，
// 于是 chat_id IS NULL 的行可以无限多条（account 级默认就会重复）⇒ 必须把 NULL 归一成
// 空串，让「account 级默认」也只允许一条。
//
// 防御性去重：换索引前先按新键去重，保留每组 id 最大的一行（业务上后写的那条
// 才是当前生效的绑定）——否则重复行会让建索引直接失败，把一次索引修正变成一次停服。
type ChannelBindingChatIDUniqueKeyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*ChannelBindingChatIDUniqueKeyMigration)(nil)

// NewChannelBindingChatIDUniqueKeyMigration 创建迁移实例
func NewChannelBindingChatIDUniqueKeyMigration(db *gorm.DB) *ChannelBindingChatIDUniqueKeyMigration {
	return &ChannelBindingChatIDUniqueKeyMigration{db: db}
}

// Version 返回版本号
func (m *ChannelBindingChatIDUniqueKeyMigration) Version() string { return "v3.70.0" }

// Name 返回迁移名称
func (m *ChannelBindingChatIDUniqueKeyMigration) Name() string {
	return "channel_agent_bindings 唯一键加入 chat_id 维度"
}

// Description 返回迁移描述
func (m *ChannelBindingChatIDUniqueKeyMigration) Description() string {
	return "把 (tenant_id, channel_type, account_id) 唯一键改为 (tenant_id, channel_type, account_id, COALESCE(chat_id,''))，" +
		"让 telegram 群组（同一账号多 chat）能各自建绑定、同时保持 account 级默认唯一；幂等可重入"
}

func (m *ChannelBindingChatIDUniqueKeyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("channel_agent_bindings") {
		return nil // 表还没建（全新库由模型标签负责），此处无物可改
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// 旧索引（两种名字都清：v3.15 建的 idx_channel_binding，
		// 以及开发库里那次一次性操作留下的 idx_channel_binding_unique）。
		for _, name := range []string{"idx_channel_binding", "idx_channel_binding_unique"} {
			if err := tx.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %q`, name)).Error; err != nil {
				return fmt.Errorf("删除旧唯一索引 %s 失败: %w", name, err)
			}
		}
		// 防御性去重：按新键（tenant_id, channel_type, account_id, COALESCE(chat_id,'')）
		// 只保留 id 最大的一行。chat_id 为 NULL 与 '' 视为同一个键。
		if err := tx.Exec(`
DELETE FROM channel_agent_bindings a USING channel_agent_bindings b
WHERE a.tenant_id = b.tenant_id
  AND a.channel_type = b.channel_type
  AND a.account_id = b.account_id
  AND COALESCE(a.chat_id, '') = COALESCE(b.chat_id, '')
  AND a.id < b.id`).Error; err != nil {
			return fmt.Errorf("channel_agent_bindings 重复行清理失败: %w", err)
		}
		// 重建：名字沿用开发库里那一个，避免同一张表上出现两个语义相同的唯一索引。
		if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_binding_unique
ON channel_agent_bindings (tenant_id, channel_type, account_id, COALESCE(chat_id, ''))`).Error; err != nil {
			return fmt.Errorf("创建含 chat_id 的唯一索引失败: %w", err)
		}
		return nil
	})
}

// Down 回滚：退到不含 chat_id 的唯一键会让 telegram 群组的绑定行建不出来
// （同一账号多 chat 会被判重），属会丢功能的降级，不做自动回滚。
func (m *ChannelBindingChatIDUniqueKeyMigration) Down(ctx context.Context) error {
	return nil
}
