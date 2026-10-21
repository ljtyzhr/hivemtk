package repository

import (
	"context"
	"fmt"

	"hivemtk-user/internal/content/model"

	"gorm.io/gorm"
)

// AIGenerationRecordRepository AI生成记录仓库接口
//
// 每个方法都收 ctx：隔离谓词由 GORM callback 从 `Statement.Context` 取租户，
// ctx 传不进来就等于没有租户——on 模式下整表 fail-closed，off 模式下靠列默认值 1。
//
// 按主键的口分两种权限宽度，且这个宽度由路由决定、不是可选过滤条件：
//   - 用户口（GetByID/UpdateFavorite/UpdateRating，挂在 auth 组与 /ai-content 别名上）
//     还要求 `user_id` 匹配：记录里存着用户输入的原文与生成结果，同租户内也是隐私。
//   - 管理口（UpdateSaved/Delete，只挂在 admin 组、由 AdminAuthMiddleware 把关角色）
//     只按租户收窄：管理员整理的是本租户的内容库，不是自己的收藏夹。
//
// 两种口命中 0 行一律报错，不能让调用方把「不是你的」读成「改好了」。
type AIGenerationRecordRepository interface {
	Create(ctx context.Context, record *model.AIGenerationRecord) error
	GetByID(ctx context.Context, id, userID uint) (*model.AIGenerationRecord, error)
	GetByMerchantAndUser(ctx context.Context, userID uint, page, pageSize int, filters map[string]any) ([]*model.AIGenerationRecord, int64, error)
	UpdateSaved(ctx context.Context, id uint, isSaved bool) error
	UpdateFavorite(ctx context.Context, id, userID uint, isFavorite bool) error
	UpdateRating(ctx context.Context, id, userID uint, rating int) error
	Delete(ctx context.Context, id uint) error
}

type aiGenerationRecordRepo struct {
	db *gorm.DB
}

// NewAIGenerationRecordRepository 创建AI生成记录仓库实例
func NewAIGenerationRecordRepository(db *gorm.DB) AIGenerationRecordRepository {
	return &aiGenerationRecordRepo{db: db}
}

func (r *aiGenerationRecordRepo) Create(ctx context.Context, record *model.AIGenerationRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *aiGenerationRecordRepo) GetByID(ctx context.Context, id, userID uint) (*model.AIGenerationRecord, error) {
	var record model.AIGenerationRecord
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *aiGenerationRecordRepo) GetByMerchantAndUser(ctx context.Context, userID uint, page, pageSize int, filters map[string]any) ([]*model.AIGenerationRecord, int64, error) {
	var records []*model.AIGenerationRecord
	var total int64

	query := r.db.WithContext(ctx).Model(&model.AIGenerationRecord{}).Where("user_id = ?", userID)

	if recordType, ok := filters["type"]; ok && recordType != "" {
		query = query.Where("type = ?", recordType)
	}
	if isSaved, ok := filters["is_saved"]; ok {
		query = query.Where("is_saved = ?", isSaved)
	}
	if isFavorite, ok := filters["is_favorite"]; ok {
		query = query.Where("is_favorite = ?", isFavorite)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&records).Error
	return records, total, err
}

func (r *aiGenerationRecordRepo) UpdateSaved(ctx context.Context, id uint, isSaved bool) error {
	return aiWriteRecordColumn(ctx, r.db, id, nil, "is_saved", isSaved)
}

func (r *aiGenerationRecordRepo) UpdateFavorite(ctx context.Context, id, userID uint, isFavorite bool) error {
	return aiWriteRecordColumn(ctx, r.db, id, &userID, "is_favorite", isFavorite)
}

func (r *aiGenerationRecordRepo) UpdateRating(ctx context.Context, id, userID uint, rating int) error {
	return aiWriteRecordColumn(ctx, r.db, id, &userID, "rating", rating)
}

// aiWriteRecordColumn 按主键写单列的三个口共用：owner 为 nil 表示管理口（只按租户收窄），
// 非 nil 表示用户口（还要求归属本人）。
// gorm 对「命中 0 行的 UPDATE」只回 `Error == nil`，于是越权写与不存在写都会被 handler
// 报成「操作成功」——用户界面上打了分，库里一个字节都没变。
func aiWriteRecordColumn(ctx context.Context, db *gorm.DB, id uint, owner *uint, column string, value any) error {
	query := db.WithContext(ctx).Model(&model.AIGenerationRecord{}).Where("id = ?", id)
	if owner != nil {
		query = query.Where("user_id = ?", *owner)
	}
	res := query.Update(column, value)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		if owner != nil {
			return fmt.Errorf("生成记录不存在（id=%d 不属于 user=%d）：本次 %s 未写入任何行", id, *owner, column)
		}
		return fmt.Errorf("生成记录不存在（id=%d 不在本租户内）：本次 %s 未写入任何行", id, column)
	}
	return nil
}

func (r *aiGenerationRecordRepo) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.AIGenerationRecord{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("生成记录不存在（id=%d 不在本租户内）：本次删除未命中任何行", id)
	}
	return nil
}

// PromptTemplateRepository 提示词模板仓库接口
//
// 模板没有归属列（租户内共享，靠 is_system 分档，写口挂在 admin 路由下），
// 所以这一侧的遮挡只有租户谓词一道；跨租户 Update/Delete 同样是 0 行，同样必须报错。
type PromptTemplateRepository interface {
	Create(ctx context.Context, template *model.PromptTemplate) error
	GetByID(ctx context.Context, id uint) (*model.PromptTemplate, error)
	ListByType(ctx context.Context, templateType string) ([]*model.PromptTemplate, error)
	GetByTypeAndName(ctx context.Context, templateType model.AIGenerationType, name string) (*model.PromptTemplate, error)
	Update(ctx context.Context, template *model.PromptTemplate) error
	Delete(ctx context.Context, id uint) error
}

type promptTemplateRepo struct {
	db *gorm.DB
}

// NewPromptTemplateRepository 创建提示词模板仓库实例
func NewPromptTemplateRepository(db *gorm.DB) PromptTemplateRepository {
	return &promptTemplateRepo{db: db}
}

func (r *promptTemplateRepo) Create(ctx context.Context, template *model.PromptTemplate) error {
	return r.db.WithContext(ctx).Create(template).Error
}

func (r *promptTemplateRepo) GetByID(ctx context.Context, id uint) (*model.PromptTemplate, error) {
	var template model.PromptTemplate
	if err := r.db.WithContext(ctx).First(&template, id).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

func (r *promptTemplateRepo) ListByType(ctx context.Context, templateType string) ([]*model.PromptTemplate, error) {
	var templates []*model.PromptTemplate

	query := r.db.WithContext(ctx).Model(&model.PromptTemplate{}).Where("status = ?", 1)

	if templateType != "" {
		query = query.Where("type = ?", templateType)
	}

	err := query.Order("is_system DESC, created_at ASC").Find(&templates).Error
	return templates, err
}

func (r *promptTemplateRepo) GetByTypeAndName(ctx context.Context, templateType model.AIGenerationType, name string) (*model.PromptTemplate, error) {
	var template model.PromptTemplate
	err := r.db.WithContext(ctx).Where("type = ? AND name = ?", templateType, name).First(&template).Error
	if err != nil {
		return nil, err
	}
	return &template, nil
}

func (r *promptTemplateRepo) Update(ctx context.Context, template *model.PromptTemplate) error {
	// 这里不能用 Save：Save 的 UPDATE 命中 0 行时（on 模式下正是「行不在本租户」这一情形）
	// 会回退成 `INSERT ... ON CONFLICT ("id") DO UPDATE SET tenant_id="excluded".tenant_id, ...`
	// （gorm v1.31.2 finisher_api.go Save 的 RowsAffected==0 分支）。冲突目标是裸主键、不含租户，
	// 于是别的租户一次「本该改不到」的 Save 反而把整行搬到自己的 tenant_id 下。
	// 改成显式列集 Updates：谓词只剩 WHERE id AND tenant_id，命中 0 行就是 0 行。
	res := r.db.WithContext(ctx).Model(&model.PromptTemplate{}).
		Where("id = ?", template.ID).
		Select("name", "type", "template", "variables", "description", "example").
		Updates(template)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("模板不存在（id=%d 不在本租户内）：本次保存未命中任何行", template.ID)
	}
	return nil
}

func (r *promptTemplateRepo) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.PromptTemplate{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("模板不存在（id=%d 不在本租户内）：本次删除未命中任何行", id)
	}
	return nil
}
