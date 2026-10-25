// dialogue_eval.go 对话级 gold 评测运行记录仓储（2026-10-25 自 service 层下沉）。
package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// DialogueEvalRepository dialogue_eval_runs 表访问。
type DialogueEvalRepository struct {
	db *gorm.DB
}

// NewDialogueEvalRepository 默认构造（全局句柄，仓库既有惯例）。
func NewDialogueEvalRepository() *DialogueEvalRepository {
	return &DialogueEvalRepository{db: GetDB()}
}

// CreateRun 落一行运行记录。
func (r *DialogueEvalRepository) CreateRun(ctx context.Context, run *model.DialogueEvalRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

// ListRuns 按时间倒序列出（limit 收敛是业务规则，留在 service）。
func (r *DialogueEvalRepository) ListRuns(ctx context.Context, limit int) ([]*model.DialogueEvalRun, error) {
	var runs []*model.DialogueEvalRun
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

// GetRun 单条详情；不存在返回 gorm.ErrRecordNotFound。
func (r *DialogueEvalRepository) GetRun(ctx context.Context, id uint) (*model.DialogueEvalRun, error) {
	var run model.DialogueEvalRun
	if err := r.db.WithContext(ctx).First(&run, id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// CreateSet 落一行评测集。
func (r *DialogueEvalRepository) CreateSet(ctx context.Context, set *model.DialogueEvalSet) error {
	return r.db.WithContext(ctx).Create(set).Error
}

// ListSets 最近的评测集（列表页不拉 Payload 大字段）。
func (r *DialogueEvalRepository) ListSets(ctx context.Context, limit int) ([]model.DialogueEvalSet, error) {
	var sets []model.DialogueEvalSet
	err := r.db.WithContext(ctx).Omit("payload").
		Order("id DESC").Limit(limit).Find(&sets).Error
	return sets, err
}

// GetSet 单条评测集（含 Payload）；不存在返回 gorm.ErrRecordNotFound。
func (r *DialogueEvalRepository) GetSet(ctx context.Context, id uint) (*model.DialogueEvalSet, error) {
	var set model.DialogueEvalSet
	if err := r.db.WithContext(ctx).First(&set, id).Error; err != nil {
		return nil, err
	}
	return &set, nil
}
