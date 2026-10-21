// Package repository 提供 AssetBundleVersionLog 的 CRUD 仓储。
//
// 方向9：资产包版本变更日志
// 文档依据：docs/企业级架构优化/资产包模式.md §五
package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// AssetBundleVersionLogRepository 资产包版本日志仓储接口
type AssetBundleVersionLogRepository interface {
	Create(ctx context.Context, m *model.AssetBundleVersionLog) error
}

type assetBundleVersionLogRepo struct {
	db *gorm.DB
}

// NewAssetBundleVersionLogRepository 构造版本日志仓储
func NewAssetBundleVersionLogRepository(db *gorm.DB) AssetBundleVersionLogRepository {
	return &assetBundleVersionLogRepo{db: db}
}

func (r *assetBundleVersionLogRepo) Create(ctx context.Context, m *model.AssetBundleVersionLog) error {
	if m == nil {
		return errors.New("asset bundle version log is nil")
	}
	return r.db.WithContext(ctx).Create(m).Error
}
