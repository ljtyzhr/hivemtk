package model

import "time"

// KnowledgeChunkParent 父块（父子分段，对标 Dify parent-child chunk）。
//
// 为什么单独一张表而不是在 knowledge_chunks 里加「父行 + is_parent 标志」：
// knowledge_chunks 是**检索与计数的对象面**——向量/关键词两条 SQL、chunk 计数、
// 发布制可见性、统计口径全都扫它。把父块塞进去要么每处加过滤（漏一处就是
// 父块当命中、计数虚高），要么让父块被向量化（检索结果重复）。
// 父块只在「命中后回拼」这一刻需要，独立表让既有查询面零改动。
//
// 生命周期：随文档删除/重切一并清理（同 document_id 一起删）。
type KnowledgeChunkParent struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID         uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	DocumentID uint64 `gorm:"index;not null" json:"document_id"`
	ProductID  string `gorm:"index;not null;default:''" json:"product_id"`

	// Content 窗口内若干子块的拼接（窗口边界按子块顺序切，不重叠）。
	Content string `gorm:"type:text;not null" json:"content"`
	// ChildCount 本父块覆盖的子块数（排查「为什么这条召回带了这么多上下文」）。
	ChildCount int `json:"child_count"`
	// TokenCount 估算 token 数（与子块同一套估算口径）。
	TokenCount int `json:"token_count"`
	CharCount  int `json:"char_count"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 父块表。
func (KnowledgeChunkParent) TableName() string { return "knowledge_chunk_parents" }
