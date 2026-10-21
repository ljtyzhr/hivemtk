package repository

import (
	"context"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoPushRecordRepository 蜘蛛推送记录仓储（append-only）
type GeoPushRecordRepository interface {
	BatchCreate(ctx context.Context, records []*model.GeoPushRecord) error
}

type geoPushRecordRepo struct{ db *gorm.DB }

func NewGeoPushRecordRepository() GeoPushRecordRepository {
	return &geoPushRecordRepo{db: _db.GetDB()}
}

// NewGeoPushRecordRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoPushRecordRepositoryWithDB(db *gorm.DB) GeoPushRecordRepository {
	return &geoPushRecordRepo{db: db}
}

// BatchCreate 一次推送的多条结果落库（按 200 行分批，避免单条 INSERT 过大）。
// tenant_id 由隔离 callback 逐行按 ctx 填充；on 模式下 ctx 未绑租户直接报错。
func (r *geoPushRecordRepo) BatchCreate(ctx context.Context, records []*model.GeoPushRecord) error {
	if len(records) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(records, 200).Error
}
