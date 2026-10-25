// kb_template.go 知识库模板（R39-K7：新用户一键建 KB）。
package model

import (
	"time"

	"gorm.io/gorm"
)

// KbTemplate 知识库模板。apply 用它一键创建真实知识库（KnowledgeBase），
// Structure 是预置结构快照（分类/示例条目建议），apply 时原样带回给前端铺底——
// 后端不解释它的 schema，条目落库仍走既有文档/FAQ 通道。
type KbTemplate struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string `gorm:"type:varchar(128);not null" json:"name"`
	Category    string `gorm:"type:varchar(50);default:general;index" json:"category"`
	Description string `gorm:"type:text" json:"description"`
	// KBType apply 时创建的知识库类型，与 KnowledgeBase.Type 同值域（faq/rag/sop）。
	KBType string `gorm:"type:varchar(16);not null" json:"kb_type"`
	// Structure 预置结构 JSON；空串表示纯空库模板。
	Structure string `gorm:"type:text" json:"structure"`
	// Status draft/published：draft 不出现在 apply 候选里。
	Status      string         `gorm:"type:varchar(16);not null;default:published" json:"status"`
	Rating      float64        `gorm:"type:decimal(3,2);default:0" json:"rating"`
	RatingCount int            `gorm:"default:0" json:"rating_count"`
	ApplyCount  int            `gorm:"default:0" json:"apply_count"`
	CreatedBy   uint           `json:"created_by"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (KbTemplate) TableName() string { return "kb_templates" }

const (
	KbTemplateStatusDraft     = "draft"
	KbTemplateStatusPublished = "published"
)
