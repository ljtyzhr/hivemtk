// Package model 未知问题回流（T1）——低置信/未命中问题沉淀、聚类、审核、写回 FAQ。
//
// 设计文档：docs/architecture/design/T1-confidence-reflux.md
// 隔离：三表均含 TenantID（tenancyidx callback 判据）。
package model

import (
	"time"

	"github.com/lib/pq"
)

// UnknownQuestionStatus 未知问题状态机
const (
	UQStatusDraft         = "draft"
	UQStatusPendingReview = "pending_review"
	UQStatusAdopted       = "adopted"
	UQStatusIgnored       = "ignored"
	UQStatusMerged        = "merged"
)

// UnknownQuestionSource 触发来源
const (
	UQSourceLowConfidence = "low_confidence"
	UQSourceHandoff       = "handoff"
	UQSourceBadCase       = "bad_case"
	UQSourceCSATNegative  = "csat_negative"
	UQSourceNoMatch       = "no_match"
)

// UnknownQuestion 低置信/未命中的用户问题沉淀（unknown_questions）
type UnknownQuestion struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	QuestionHash string `gorm:"column:question_hash;size:64;not null;uniqueIndex:uk_uq_tenant_hash,priority:2" json:"question_hash"` // sha256(normalize(question))
	Question     string `gorm:"column:question;type:text;not null" json:"question"`
	ClusterID    int64  `gorm:"column:cluster_id;default:0;index:idx_uq_cluster" json:"cluster_id"`

	Status     string `gorm:"column:status;size:32;not null;default:'draft';index:idx_uq_tenant_status,priority:2" json:"status"`
	Source     string `gorm:"column:source;size:32;not null" json:"source"`
	SourceID   string `gorm:"column:source_id;size:64;default:''" json:"source_id"`
	SessionID  string `gorm:"column:session_id;size:128;default:''" json:"session_id"`
	CustomerID string `gorm:"column:customer_id;size:64;default:''" json:"customer_id"`
	AgentID    uint   `gorm:"column:agent_id;default:0;index:idx_uq_tenant_status,priority:3" json:"agent_id"`
	IntentType string `gorm:"column:intent_type;size:64;default:''" json:"intent_type"`
	// LowConfScore 触发时的聚合置信度快照
	LowConfScore float64 `gorm:"column:low_conf_score;type:decimal(5,4);default:0" json:"low_conf_score"`

	Frequency      int        `gorm:"column:frequency;not null;default:1" json:"frequency"`
	FirstSeenAt    time.Time  `gorm:"column:first_seen_at;not null" json:"first_seen_at"`
	LastSeenAt     time.Time  `gorm:"column:last_seen_at;not null;index:idx_uq_last_seen" json:"last_seen_at"`
	MergedTargetID int64      `gorm:"column:merged_target_id;default:0" json:"merged_target_id"`
	AdoptedFaqID   uint       `gorm:"column:adopted_faq_id;default:0" json:"adopted_faq_id"`
	ReviewedBy     string     `gorm:"column:reviewed_by;size:64;default:''" json:"reviewed_by"`
	ReviewedAt     *time.Time `gorm:"column:reviewed_at" json:"reviewed_at,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName 表名
func (UnknownQuestion) TableName() string { return "unknown_questions" }

// UnknownQuestionCluster 未知问题聚类簇（unknown_question_clusters）
type UnknownQuestionCluster struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterHash string `gorm:"column:cluster_hash;size:64;not null;uniqueIndex:uk_uqc_tenant_hash,priority:2" json:"cluster_hash"` // 簇代表问题 hash
	Title       string `gorm:"column:title;size:255;not null" json:"title"`                                                        // 代表问句
	AgentID     uint   `gorm:"column:agent_id;default:0;index" json:"agent_id"`
	Status      string `gorm:"column:status;size:32;not null;default:'pending_review';index:idx_uqc_status" json:"status"`
	// QuestionCount / TotalFrequency 排序与规模展示
	QuestionCount   int            `gorm:"column:question_count;not null;default:0" json:"question_count"`
	TotalFrequency  int            `gorm:"column:total_frequency;not null;default:0" json:"total_frequency"`
	SampleQuestions pq.StringArray `gorm:"column:sample_questions;type:text[];default:'{}'" json:"sample_questions"`
	AdoptedFaqID    uint           `gorm:"column:adopted_faq_id;default:0" json:"adopted_faq_id"`
	ReviewedBy      string         `gorm:"column:reviewed_by;size:64;default:''" json:"reviewed_by"`
	ReviewedAt      *time.Time     `gorm:"column:reviewed_at" json:"reviewed_at,omitempty"`
	CreatedAt       time.Time      `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName 表名
func (UnknownQuestionCluster) TableName() string { return "unknown_question_clusters" }

// UnknownQuestionReviewLog 审核操作流水（unknown_question_review_logs，仅追加）
type UnknownQuestionReviewLog struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TargetType string    `gorm:"column:target_type;size:16;not null;index:idx_uqrl_target,priority:2" json:"target_type"` // question | cluster
	TargetID   int64     `gorm:"column:target_id;not null;index:idx_uqrl_target,priority:3" json:"target_id"`
	Action     string    `gorm:"column:action;size:32;not null" json:"action"` // submit | adopt | ignore | merge | reject_detail
	FromStatus string    `gorm:"column:from_status;size:32;default:''" json:"from_status"`
	ToStatus   string    `gorm:"column:to_status;size:32;default:''" json:"to_status"`
	OperatorID string    `gorm:"column:operator_id;size:64;not null" json:"operator_id"`
	Detail     string    `gorm:"column:detail;type:text;default:''" json:"detail"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;index:idx_uqrl_target,priority:4" json:"created_at"`
}

// TableName 表名
func (UnknownQuestionReviewLog) TableName() string { return "unknown_question_review_logs" }
