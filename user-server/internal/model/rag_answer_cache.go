package model

import "time"

// RAGAnswerCache 语义答案缓存行（表 rag_answer_cache）。
// query_vector 是 vector(1024)，读写走 pgvector 运算符，见 internal/aiagent/rag/cache。
// 检索永远带 (tenant_id, kb_id, prompt_version) 三元组，这三列共用一个复合索引。
type RAGAnswerCache struct {
	TenantID      uint      `gorm:"column:tenant_id;not null;default:1;index:idx_rag_answer_cache_kb_prompt,priority:1"`
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	KBID          string    `gorm:"column:kb_id;type:varchar(64);not null;index:idx_rag_answer_cache_kb_prompt,priority:2"`
	PromptVersion string    `gorm:"column:prompt_version;type:varchar(64);not null;index:idx_rag_answer_cache_kb_prompt,priority:3"`
	QueryVector   string    `gorm:"column:query_vector;type:vector(1024);not null"`
	Answer        string    `gorm:"column:answer;type:text;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	KBUpdatedAt   time.Time `gorm:"column:kb_updated_at;not null"`
}

// TableName GORM 表名
func (RAGAnswerCache) TableName() string { return "rag_answer_cache" }
