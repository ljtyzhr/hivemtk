package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoJobRunRepository 定时任务运行历史仓储
type GeoJobRunRepository interface {
	Create(ctx context.Context, run *model.GeoJobRun) error
	Finish(ctx context.Context, id uint, status, summary, errMsg string, durationMs int64) error
	List(ctx context.Context, jobName string, page, limit int) ([]*model.GeoJobRun, int64, error)
	Latest(ctx context.Context, jobName string) (*model.GeoJobRun, error)
	// FailStaleRunning 启动兜底：进程重启后残留的 running 记录置为 failed
	FailStaleRunning(ctx context.Context) error
	// DeleteBefore 清理历史（保留窗口控制）
	DeleteBefore(ctx context.Context, before time.Time) error
}

type geoJobRunRepo struct {
	db *gorm.DB
}

func NewGeoJobRunRepository() GeoJobRunRepository {
	return &geoJobRunRepo{db: _db.GetDB()}
}

// NewGeoJobRunRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoJobRunRepositoryWithDB(db *gorm.DB) GeoJobRunRepository {
	return &geoJobRunRepo{db: db}
}

func (r *geoJobRunRepo) Create(ctx context.Context, run *model.GeoJobRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *geoJobRunRepo) Finish(ctx context.Context, id uint, status, summary, errMsg string, durationMs int64) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.GeoJobRun{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":      status,
			"summary":     summary,
			"error":       errMsg,
			"finished_at": &now,
			"duration_ms": durationMs,
		})
	if res.Error != nil {
		return res.Error
	}
	// 0 行说明记录已被清理或不属于本租户：不能当作回写成功。
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *geoJobRunRepo) List(ctx context.Context, jobName string, page, limit int) ([]*model.GeoJobRun, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Model(&model.GeoJobRun{})
	if jobName != "" {
		q = q.Where("job_name = ?", jobName)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.GeoJobRun
	if err := q.Order("id DESC").Offset((page - 1) * limit).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *geoJobRunRepo) Latest(ctx context.Context, jobName string) (*model.GeoJobRun, error) {
	var run model.GeoJobRun
	err := r.db.WithContext(ctx).
		Where("job_name = ?", jobName).
		Order("id DESC").First(&run).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *geoJobRunRepo) FailStaleRunning(ctx context.Context) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.GeoJobRun{}).
		Where("status = ?", "running").
		Updates(map[string]any{
			"status":      "failed",
			"error":       "进程重启，运行中断",
			"finished_at": &now,
		}).Error
}

func (r *geoJobRunRepo) DeleteBefore(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&model.GeoJobRun{}).Error
}
