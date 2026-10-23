// reach_compliance_log_repo.go 合规审计日志批量落库仓储（五层 L5）
package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// ReachComplianceLogRepository reach_compliance_log 批量写入收口
type ReachComplianceLogRepository struct {
	db *gorm.DB
}

// NewReachComplianceLogRepository 构造
func NewReachComplianceLogRepository(db *gorm.DB) *ReachComplianceLogRepository {
	return &ReachComplianceLogRepository{db: db}
}

// CreateInBatches 批量写入合规日志。
//
// ctx **必须带租户**：reach_compliance_log 是租户表。刷盘侧按行里抄好的 TenantID
// 分组、每组 `tenant.OnlyTenant` 显式绑定后再调用本方法 —— 传后台 ctx
// （context.Background()）会让 on 模式下的隔离 callback 把整批 fail-closed 拒掉。
//
// db 为 nil 时返回 nil 是**刻意的降级**（`InitComplianceAuditLogger(nil)` 的
// 向后兼容路径：只记 WARN、不落库），不是漏判 —— 那一路根本不会启动 flushLoop。
// 写侧的"句柄缺失必须报错"针对的是**声明要落库**的调用方，见
// delayed_outbound_repo.go 里 errDelayedOutboundUnavailable 的说明。
func (r *ReachComplianceLogRepository) CreateInBatches(ctx context.Context, batch []*model.ReachComplianceLog, batchSize int) error {
	if r.db == nil || len(batch) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(batch, batchSize).Error
}
