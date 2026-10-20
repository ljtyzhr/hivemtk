package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hivemtk-user/internal/browser_automation/model"
)

// BrowserOutreachDedupeRepository 跨任务触达去重仓储。
type BrowserOutreachDedupeRepository interface {
	// FindOutreachHit 按四元键查命中；未命中返回 (nil, nil)。
	FindOutreachHit(ctx context.Context, platform, targetURL, action, copyHash string) (*model.BrowserOutreachDedupe, error)
	// RecordOutreachSend 记录一次发送；唯一冲突幂等（不报错）。
	RecordOutreachSend(ctx context.Context, rec *model.BrowserOutreachDedupe) error
}

type browserOutreachDedupeRepository struct {
	db *gorm.DB
}

// NewBrowserOutreachDedupeRepositoryWithDB 装配注入（路由与测试共用这一条构造口）。
//
// 曾经另有一条无参的 `NewBrowserOutreachDedupeRepository()`，内部取 `_db.GetDB()` 全局句柄：
// 零调用方（本域装配在 `router/browser_automation_routes.go:36` 显式传 gormDB），
// 留着等于在本该纯注入的仓储面上再开一条「谁都能不接 ctx/db 直接摸库」的后门，
// 与「DI 在装配层」这条铁律相反 —— 已删（#7 四关 4.4）。
func NewBrowserOutreachDedupeRepositoryWithDB(db *gorm.DB) BrowserOutreachDedupeRepository {
	return &browserOutreachDedupeRepository{db: db}
}

// FindOutreachHit 按四元键查命中。
func (r *browserOutreachDedupeRepository) FindOutreachHit(ctx context.Context, platform, targetURL, action, copyHash string) (*model.BrowserOutreachDedupe, error) {
	var row model.BrowserOutreachDedupe
	err := r.db.WithContext(ctx).
		Where("platform = ? AND target_url = ? AND action = ? AND copy_hash = ?", platform, targetURL, action, copyHash).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// RecordOutreachSend 记录一次发送；唯一冲突幂等。
func (r *browserOutreachDedupeRepository) RecordOutreachSend(ctx context.Context, rec *model.BrowserOutreachDedupe) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(rec).Error
}
