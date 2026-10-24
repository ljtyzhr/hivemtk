package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// ReachPipelineStats 触达统计聚合结果（一次性返回 Pipeline + Job 全量统计）
type ReachPipelineStats struct {
	TotalPipelines  int64
	ActivePipelines int64
	PausedPipelines int64

	TotalJobs       int64
	PendingJobs     int64
	RunningJobs     int64
	SuccessJobs     int64
	FailedJobs      int64
	RateLimitedJobs int64
	CanceledJobs    int64
}

type ReachPipelineRepository struct {
	db *gorm.DB
}

func NewReachPipelineRepository(db *gorm.DB) *ReachPipelineRepository {
	return &ReachPipelineRepository{db: db}
}

// Available 底层 db 是否可用（纯内存判断，不涉及 DB 操作，故不接收 ctx 参数）。
func (r *ReachPipelineRepository) Available() bool {
	return r != nil && r.db != nil
}

func (r *ReachPipelineRepository) CreatePipeline(ctx context.Context, pipe *model.ReachPipeline) error {
	if r == nil || r.db == nil {
		return errors.New("reach pipeline repository not initialized")
	}
	return r.db.WithContext(ctx).Create(pipe).Error
}

func (r *ReachPipelineRepository) FindPipelineByID(ctx context.Context, id uint) (*model.ReachPipeline, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("reach pipeline repository not initialized")
	}
	var pipe model.ReachPipeline
	if err := r.db.WithContext(ctx).First(&pipe, id).Error; err != nil {
		return nil, err
	}
	return &pipe, nil
}

// SavePipeline 保存 Pipeline 全字段。
// 不用 Save：Save 命中 0 行会回退成裸主键 upsert，跨租户 Save 会把整行搬到调用方租户下；
// 这里用列 map + WHERE id（租户谓词由隔离 callback 注入），0 行即 NotFound。
func (r *ReachPipelineRepository) SavePipeline(ctx context.Context, pipe *model.ReachPipeline) error {
	if r == nil || r.db == nil {
		return errors.New("reach pipeline repository not initialized")
	}
	res := r.db.WithContext(ctx).Model(&model.ReachPipeline{}).
		Where("id = ?", pipe.ID).
		Updates(map[string]any{
			"name":          pipe.Name,
			"description":   pipe.Description,
			"channel":       pipe.Channel,
			"steps":         pipe.Steps,
			"retry_policy":  pipe.RetryPolicy,
			"rate_limit":    pipe.RateLimit,
			"status":        pipe.Status,
			"version":       pipe.Version,
			"total_runs":    pipe.TotalRuns,
			"total_success": pipe.TotalSuccess,
			"total_failure": pipe.TotalFailure,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ReachPipelineRepository) ListPipelines(ctx context.Context, channel, status string, page, pageSize int) ([]model.ReachPipeline, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("reach pipeline repository not initialized")
	}
	q := r.db.WithContext(ctx).Model(&model.ReachPipeline{})
	if channel != "" {
		q = q.Where("channel = ?", channel)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.ReachPipeline
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

func (r *ReachPipelineRepository) DeletePipeline(ctx context.Context, id uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("reach pipeline repository not initialized")
	}
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.ReachPipeline{})
	return res.RowsAffected, res.Error
}

// DeleteJobsByPipeline 级联删除指定 Pipeline 下的所有任务。
// 两模型均无软删除，不级联清理会留下孤儿任务（ExecuteJob 时报 pipeline not found）。
func (r *ReachPipelineRepository) DeleteJobsByPipeline(ctx context.Context, pipelineID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("reach pipeline repository not initialized")
	}
	res := r.db.WithContext(ctx).Where("pipeline_id = ?", pipelineID).Delete(&model.ReachJob{})
	return res.RowsAffected, res.Error
}

// ListDueJobs 列出到期应执行的任务（state IN pending/retrying/rate_limited 且 next_run_at <= now），
// 按 next_run_at 升序，单次最多 200 条，供后台调度器消费。
func (r *ReachPipelineRepository) ListDueJobs(ctx context.Context, now time.Time) ([]model.ReachJob, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("reach pipeline repository not initialized")
	}
	var jobs []model.ReachJob
	err := r.db.WithContext(ctx).
		Where("state IN ?", []string{"pending", "retrying", "rate_limited"}).
		Where("next_run_at <= ?", now).
		Order("next_run_at ASC").
		Limit(200).
		Find(&jobs).Error
	return jobs, err
}

// ClaimJob 原子抢占任务：仅当任务处于可执行状态时置为 running，返回 true 表示抢占成功。
// 用于调度器与手动触发之间的并发去重。
func (r *ReachPipelineRepository) ClaimJob(ctx context.Context, id uint) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("reach pipeline repository not initialized")
	}
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&model.ReachJob{}).
		Where("id = ? AND state IN ?", id, []string{"pending", "retrying", "rate_limited"}).
		Updates(map[string]any{
			"state":      "running",
			"started_at": &now,
		})
	return res.RowsAffected > 0, res.Error
}

// ResetStuckJobs 把卡在 running 超过 olderThan 的任务重置为 pending，交调度器重新消费
// （进程崩溃/调度器异常退出会让任务永久停留 running）。
func (r *ReachPipelineRepository) ResetStuckJobs(ctx context.Context, olderThan time.Duration) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("reach pipeline repository not initialized")
	}
	cutoff := time.Now().Add(-olderThan)
	res := r.db.WithContext(ctx).
		Model(&model.ReachJob{}).
		Where("state = ? AND updated_at < ?", "running", cutoff).
		Updates(map[string]any{
			"state":       "pending",
			"next_run_at": time.Now(),
		})
	return res.RowsAffected, res.Error
}

// UpdatePipelineStatus 更新 Pipeline 状态（active/paused/archived）；0 行命中报 NotFound。
func (r *ReachPipelineRepository) UpdatePipelineStatus(ctx context.Context, id uint, status string) error {
	if r == nil || r.db == nil {
		return errors.New("reach pipeline repository not initialized")
	}
	res := r.db.WithContext(ctx).Model(&model.ReachPipeline{}).
		Where("id = ?", id).
		Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// IncrementPipelineField 给 Pipeline 的指定数值字段自增 delta（用于 total_runs/total_success/total_failure）。
func (r *ReachPipelineRepository) IncrementPipelineField(ctx context.Context, id uint, field string, delta int) error {
	if r == nil || r.db == nil {
		return errors.New("reach pipeline repository not initialized")
	}
	return r.db.WithContext(ctx).Model(&model.ReachPipeline{}).
		Where("id = ?", id).
		Update(field, gorm.Expr(field+" + ?", delta)).Error
}

func (r *ReachPipelineRepository) CreateJob(ctx context.Context, job *model.ReachJob) error {
	if r == nil || r.db == nil {
		return errors.New("reach pipeline repository not initialized")
	}
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *ReachPipelineRepository) FindJobByID(ctx context.Context, id uint) (*model.ReachJob, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("reach pipeline repository not initialized")
	}
	var job model.ReachJob
	if err := r.db.WithContext(ctx).First(&job, id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *ReachPipelineRepository) ListJobs(ctx context.Context, channel, state string, page, pageSize int) ([]model.ReachJob, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("reach pipeline repository not initialized")
	}
	q := r.db.WithContext(ctx).Model(&model.ReachJob{})
	if channel != "" {
		q = q.Where("channel = ?", channel)
	}
	if state != "" {
		q = q.Where("state = ?", state)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.ReachJob
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

// UpdateJobStateWithCond 在指定 state 范围内更新任务字段，返回受影响行数
// （用于 CancelJob 与 RetryJob）。
func (r *ReachPipelineRepository) UpdateJobStateWithCond(ctx context.Context, id uint, fromStates []string, updates map[string]any) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("reach pipeline repository not initialized")
	}
	res := r.db.WithContext(ctx).Model(&model.ReachJob{}).
		Where("id = ? AND state IN ?", id, fromStates).
		Updates(updates)
	return res.RowsAffected, res.Error
}

// SaveJob 保存任务全字段（用于 ExecuteJob 内多次回写状态）。
// 与 SavePipeline 同理：列 map 代替 Save，0 行命中即 NotFound。
func (r *ReachPipelineRepository) SaveJob(ctx context.Context, job *model.ReachJob) error {
	if r == nil || r.db == nil {
		return errors.New("reach pipeline repository not initialized")
	}
	res := r.db.WithContext(ctx).Model(&model.ReachJob{}).
		Where("id = ?", job.ID).
		Updates(map[string]any{
			"pipeline_id":   job.PipelineID,
			"channel":       job.Channel,
			"customer_id":   job.CustomerID,
			"account_id":    job.AccountID,
			"payload":       job.Payload,
			"state":         job.State,
			"current_step":  job.CurrentStep,
			"step_results":  job.StepResults,
			"retry_count":   job.RetryCount,
			"max_retry":     job.MaxRetry,
			"next_run_at":   job.NextRunAt,
			"started_at":    job.StartedAt,
			"completed_at":  job.CompletedAt,
			"error_message": job.ErrorMessage,
			"duration_ms":   job.DurationMs,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// TouchRunningJob 刷新运行中任务的 updated_at，作为执行存活心跳。
// 仅当仍处于 running 时才更新：与 ResetStuckJobs 配合，保证仍在执行的任务不被误判为
// 卡死而重复派发（重复触达竞态）。
func (r *ReachPipelineRepository) TouchRunningJob(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return errors.New("reach pipeline repository not initialized")
	}
	return r.db.WithContext(ctx).Model(&model.ReachJob{}).
		Where("id = ? AND state = ?", id, "running").
		Update("updated_at", time.Now()).Error
}

// GetScriptContent 按 ID 在 script_templates / script_libraries 表中查询 content，
// 优先前者，两张表均无命中返回错误。
func (r *ReachPipelineRepository) GetScriptContent(ctx context.Context, templateID string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("reach pipeline repository not initialized")
	}

	var st struct {
		Content string `gorm:"column:content"`
	}
	if err := r.db.WithContext(ctx).Table("script_templates").Select("content").Where("id = ?", templateID).Scan(&st).Error; err == nil && st.Content != "" {
		return st.Content, nil
	}

	var sl struct {
		Content string `gorm:"column:content"`
	}
	if err := r.db.WithContext(ctx).Table("script_library").Select("content").Where("id = ?", templateID).Scan(&sl).Error; err == nil && sl.Content != "" {
		return sl.Content, nil
	}
	return "", fmt.Errorf("template %s not found", templateID)
}

// GetStats 一次性返回 Pipeline + Job 全量统计（service 层负责映射为对外 map）。
func (r *ReachPipelineRepository) GetStats(ctx context.Context) (*ReachPipelineStats, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("reach pipeline repository not initialized")
	}
	stats := &ReachPipelineStats{}

	if err := r.db.WithContext(ctx).Model(&model.ReachPipeline{}).Count(&stats.TotalPipelines).Error; err != nil {
		return nil, fmt.Errorf("count pipelines: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ReachPipeline{}).Where("status = ?", "active").Count(&stats.ActivePipelines).Error; err != nil {
		return nil, fmt.Errorf("count active pipelines: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ReachPipeline{}).Where("status = ?", "paused").Count(&stats.PausedPipelines).Error; err != nil {
		return nil, fmt.Errorf("count paused pipelines: %w", err)
	}

	if err := r.db.WithContext(ctx).Model(&model.ReachJob{}).Count(&stats.TotalJobs).Error; err != nil {
		return nil, fmt.Errorf("count jobs: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ReachJob{}).Where("state = ?", "pending").Count(&stats.PendingJobs).Error; err != nil {
		return nil, fmt.Errorf("count pending jobs: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ReachJob{}).Where("state = ?", "running").Count(&stats.RunningJobs).Error; err != nil {
		return nil, fmt.Errorf("count running jobs: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ReachJob{}).Where("state = ?", "success").Count(&stats.SuccessJobs).Error; err != nil {
		return nil, fmt.Errorf("count success jobs: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ReachJob{}).Where("state = ?", "failed").Count(&stats.FailedJobs).Error; err != nil {
		return nil, fmt.Errorf("count failed jobs: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ReachJob{}).Where("state = ?", "rate_limited").Count(&stats.RateLimitedJobs).Error; err != nil {
		return nil, fmt.Errorf("count rate_limited jobs: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ReachJob{}).Where("state = ?", "canceled").Count(&stats.CanceledJobs).Error; err != nil {
		return nil, fmt.Errorf("count canceled jobs: %w", err)
	}
	return stats, nil
}
