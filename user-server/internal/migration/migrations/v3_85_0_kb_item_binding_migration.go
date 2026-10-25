package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// KBItemBindingMigration v3.85.0（P0-2/P0-3/D-8 方案 A：kb_item_bindings 成员表）：
// 知识库与内容（FAQ/SOP/文档）之间的成员关系此前不存在——内容表无 kb_id 列、
// 按 owner agent 反推的交集为空，前端抽屉把 kb_id 当查询参数被全局 List 静默忽略，
// rag 分支请求的 /api/knowledge 恒 404。本迁移建 kb_item_bindings 成员表，
// 把"归属"建成独立的多对多绑定：UNIQUE(kb_id, item_type, item_id) 经 tenancyidx
// callback 复合化为 (tenant_id, ...)，支持一条内容属多个 KB（shared 语义）。
//
// 无回填：存量内容与 KB 的 owner agent 交集≈0，绑定表初始为空是预期状态，
// 由管理端点（POST /api/knowledge-bases/:id/items）显式登记。
//
// 幂等可重入：AutoMigrate 建表，重复执行为空操作。
type KBItemBindingMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*KBItemBindingMigration)(nil)

// NewKBItemBindingMigration 构造
func NewKBItemBindingMigration(db *gorm.DB) *KBItemBindingMigration {
	return &KBItemBindingMigration{db: db}
}

func (m *KBItemBindingMigration) Version() string { return "v3.85.0" }

func (m *KBItemBindingMigration) Name() string { return "kb_item_bindings 成员表" }

func (m *KBItemBindingMigration) Description() string {
	return "知识库成员绑定表（P0-2/P0-3/D-8 方案 A）：一条内容（faq/sop/rag）可绑定进" +
		"一个或多个知识库，UNIQUE(kb_id,item_type,item_id) 租户内唯一；无回填，" +
		"初始为空是预期状态，由绑定管理端点显式登记"
}

func (m *KBItemBindingMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if err := m.db.WithContext(ctx).AutoMigrate(&model.KBItemBinding{}); err != nil {
		return fmt.Errorf("automigrate kb_item_bindings: %w", err)
	}
	return nil
}

func (m *KBItemBindingMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	// 绑定行是运营显式登记的归属事实，回滚即数据丢失，不允许
	declineTableDrop(m.Version(), "kb_item_bindings")
	return nil
}
