package model

import (
	"time"

	"gorm.io/gorm"
)

// KBItem 绑定成员的内容类型枚举（P0-2/P0-3/D-8 方案 A：kb_item_bindings 成员表）。
// 只收 faq|sop|rag 三种：与 KBDrawer 预览三分支、以及知识库现有内容域一一对应。
const (
	KBItemFAQ = "faq"
	KBItemSOP = "sop"
	KBItemRag = "rag"
)

// IsValidKBItemType 判定 item_type 是否合法。
func IsValidKBItemType(t string) bool {
	switch t {
	case KBItemFAQ, KBItemSOP, KBItemRag:
		return true
	}
	return false
}

// KBItemBinding 知识库成员绑定：一条内容（FAQ/SOP/文档）属于一个或多个知识库。
//
// 立项（P0-2/P0-3/D-8，2026-10-23 实施规划 §2.2）：知识库与内容之间此前没有成员关系——
// 内容表无 kb_id 列、按 owner agent 反推的交集为空（文档与 KB owner_agent_id 交集 0），
// 前端抽屉把 kb_id 当查询参数传给全局 List 被静默忽略，rag 分支请求的 /api/knowledge 恒 404。
// 方案 A（成员表）而非 B（内容表加列）：12 个 KB 里 5 个是 shared，单外键会堵死
// "一条内容属多个 KB"的共享语义；且不碰 1.3 万行的大表，允许"未分配"长期存在。
//
// 多态 item_id 无数据库级外键：服务层绑定前校验同租户 + 内容存在；删除内容时
// 由服务层清理绑定（见 KBItemBindingService.DeleteBindingsForItem）。
type KBItemBinding struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`
	ID       uint `gorm:"primaryKey;autoIncrement" json:"id"`

	KBID     uint   `gorm:"not null;uniqueIndex:idx_kb_item_unique,priority:1;index" json:"kb_id"`
	ItemType string `gorm:"type:varchar(8);not null;uniqueIndex:idx_kb_item_unique,priority:2;index" json:"item_type"`
	ItemID   uint64 `gorm:"not null;uniqueIndex:idx_kb_item_unique,priority:3;index" json:"item_id"`
	// BoundBy 登记谁做的绑定（管理端操作人标识），审计用，可空。
	BoundBy   string         `gorm:"type:varchar(64)" json:"bound_by,omitempty"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName GORM 表名
func (KBItemBinding) TableName() string { return "kb_item_bindings" }
