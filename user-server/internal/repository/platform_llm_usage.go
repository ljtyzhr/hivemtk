// platform_llm_usage.go 平台垫资模型的本地镜像与用量落库仓储
// （2026-10-25 自 service/llm_platform_usage.go 下沉，L3 铁律）。
package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrDBNotReady 全局句柄尚未装配（早期启动/单测）。调用方（用量 sink）把它
// 当「本轮跳过」处理，不是缺陷。
var ErrDBNotReady = errors.New("db handle not ready")

// PlatformLLMUsageRepository 平台用量明细 + provider 镜像仓储。
type PlatformLLMUsageRepository struct {
	db *gorm.DB
}

// NewPlatformLLMUsageRepository 默认构造（全局句柄，仓库既有惯例）。
func NewPlatformLLMUsageRepository() *PlatformLLMUsageRepository {
	return &PlatformLLMUsageRepository{db: GetDB()}
}

// NewPlatformLLMUsageRepositoryWithDB 用测试/装配注入的连接构造。
func NewPlatformLLMUsageRepositoryWithDB(db *gorm.DB) *PlatformLLMUsageRepository {
	return &PlatformLLMUsageRepository{db: db}
}

// InsertUsageRecord 幂等落一条用量明细：trace_id 撞唯一索引就跳过
// （重试/重放不重复记账——扣费权威在平台侧，这里只做镜像）。
func (r *PlatformLLMUsageRepository) InsertUsageRecord(ctx context.Context, rec *model.PlatformLLMUsageRecord) error {
	if r.db == nil {
		return ErrDBNotReady
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(rec).Error
}

// UpsertProviders 按 (tenant_id, name) upsert 平台下发的 provider 行。
//
// 冲突目标必须与库里实际索引形状一致：(tenant_id, name)——tenancyidx.Normalize
// 在框架层把租户表的唯一索引原位复合化，写 ON CONFLICT (name) 会 42P10 报
// 「no unique or exclusion constraint matching the ON CONFLICT specification」。
// 只更新平台下发的元信息与 source，**不动 api_key**——商户自己填的密钥要留着，
// 否则 provider 从 platform 转回 self 就废了。
func (r *PlatformLLMUsageRepository) UpsertProviders(ctx context.Context, rows []model.LLMProvider) (int, error) {
	if r.db == nil {
		return 0, ErrDBNotReady
	}
	now := time.Now()
	n := 0
	var firstErr error
	for _, row := range rows {
		row.UpdatedAt = now
		row.CreatedAt = now
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"display_name", "model", "api_type", "vendor",
				"source", "plan_code", "enabled", "updated_at",
			}),
		}).Create(&row).Error; err != nil {
			// 逐行容错（原 savePlatformProviders 语义）：单行失败不阻断整批，
			// 逐行留痕（观测口径与原实现一致），另返回首个错误，n 只计成功行。
			logger.Warnf("[llm-provider] 平台 provider 落库失败 %s: %v", row.Name, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		n++
	}
	return n, firstErr
}
