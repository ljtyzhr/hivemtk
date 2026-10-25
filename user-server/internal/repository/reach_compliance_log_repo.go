// reach_compliance_log_repo.go 合规审计日志批量落库仓储。
package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

type ReachComplianceLogRepository struct {
	db *gorm.DB
}

// NewReachComplianceLogRepositoryDefault 默认构造（全局句柄；与传 GetDB() 同语义）。
func NewReachComplianceLogRepositoryDefault() *ReachComplianceLogRepository {
	return NewReachComplianceLogRepository(GetDB())
}

func NewReachComplianceLogRepository(db *gorm.DB) *ReachComplianceLogRepository {
	return &ReachComplianceLogRepository{db: db}
}

// CreateInBatches 批量写入合规日志。
// ctx 必须带租户（reach_compliance_log 是租户表），传后台 ctx 会被隔离 callback fail-closed 拒掉。
// db 为 nil 时静默返回 nil 是刻意的降级（只记 WARN、不落库，那一路不会启动 flushLoop）。
func (r *ReachComplianceLogRepository) CreateInBatches(ctx context.Context, batch []*model.ReachComplianceLog, batchSize int) error {
	if r.db == nil || len(batch) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(batch, batchSize).Error
}
