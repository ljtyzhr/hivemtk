package model

import "time"

// DialogueEvalSet 导入的对话评测集（P0-1 CSV 批量导入的落点）。
//
// 为什么单独一张表而不复用 dialogue_eval_runs.Details：runs 是「跑出来的读数」，
// 本表是「跑之前要评的题」——两者生命周期相反（题被反复评、读数只增不改），
// 混在一张表里会让「这轮跑了多少条」和「这套题有多少条」永远对不上。
//
// Payload 存规范 JSON（[]eval.DialogueSample）：CSV 只是运营入口，落库用 JSON
// 保留多回合结构；导出时再从 JSON 生成 CSV，保证「导入→导出」逐字节稳定。
type DialogueEvalSet struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID uint `gorm:"primaryKey;autoIncrement" json:"id"`

	// Name 评测集名（运营填写，非唯一——同名多版本靠 ID 区分）。
	Name string `gorm:"type:varchar(128);index" json:"name"`

	// SampleCount 样本条数（与 Payload 数组长度一致，冗余列用于列表页免解析）。
	SampleCount int `json:"sample_count"`

	// Payload 规范 JSON 样本数组。
	Payload string `gorm:"type:text" json:"payload,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 对话评测集表。
func (DialogueEvalSet) TableName() string { return "dialogue_eval_sets" }
