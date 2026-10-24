package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// sopListLimit SOP 列表上限：调度/日度迭代全量扫描，封顶防止无界读。
const sopListLimit = 500

// SopAgentRepository SOP 智能体仓储
type SopAgentRepository struct {
	db *gorm.DB
}

func NewSopAgentRepository(db *gorm.DB) *SopAgentRepository {
	return &SopAgentRepository{db: db}
}

func (r *SopAgentRepository) ListActiveByTriggerType(ctx context.Context, triggerType string) ([]model.SOPAgent, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var list []model.SOPAgent
	if err := r.db.WithContext(ctx).
		Where("is_active = ? AND trigger_type = ?", true, triggerType).
		Limit(sopListLimit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateTriggerConfig 按 ID 更新 trigger_config 字段
func (r *SopAgentRepository) UpdateTriggerConfig(ctx context.Context, id uint, triggerConfig string) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.SOPAgent{}).
		Where("id = ?", id).
		Update("trigger_config", triggerConfig).Error
}

// ListByUseBandit 列出启用/禁用 Bandit 流量分配的 SOP（FeedbackLoopCron 日度迭代用）。
func (r *SopAgentRepository) ListByUseBandit(ctx context.Context, useBandit bool) ([]model.SOPAgent, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var list []model.SOPAgent
	if err := r.db.WithContext(ctx).
		Where("use_bandit = ?", useBandit).
		Limit(sopListLimit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// SopExecutionRepository SOP 执行记录仓储
type SopExecutionRepository struct {
	db *gorm.DB
}

func NewSopExecutionRepository(db *gorm.DB) *SopExecutionRepository {
	return &SopExecutionRepository{db: db}
}

// CountRunningBySOPAndCustomer 统计指定 SOP 对客户的 running 执行数（意图触发去重用）
func (r *SopExecutionRepository) CountRunningBySOPAndCustomer(ctx context.Context, sopID uint, customerID, runningStatus string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop execution repository not initialized")
	}
	var n int64
	err := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("sop_id = ? AND customer_id = ? AND status = ?", sopID, customerID, runningStatus).
		Count(&n).Error
	return n, err
}

// CleanupStuck 清理卡死的执行（started_at 早于 threshold 且 status=runningStatus 的记录批量标记为 failedStatus）
// 返回受影响行数
func (r *SopExecutionRepository) CleanupStuck(ctx context.Context, threshold time.Time, runningStatus, failedStatus string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("status = ? AND started_at < ?", runningStatus, threshold).
		Updates(map[string]any{
			"status":        failedStatus,
			"error_message": "执行超时，自动标记失败",
			"completed_at":  time.Now(),
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// CountBySOPIDAndStatus 统计某 SOP 指定状态的执行数
func (r *SopExecutionRepository) CountBySOPIDAndStatus(ctx context.Context, sopID uint, status string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop execution repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("sop_id = ? AND status = ?", sopID, status).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountBySOPIDAndCustomerIDAndStatus 统计某 SOP 在指定客户下的指定状态执行数
// 用于 SOP 去重：同一客户 + 同一 SOP + running 状态 → 跳过新执行
func (r *SopExecutionRepository) CountBySOPIDAndCustomerIDAndStatus(ctx context.Context, sopID uint, customerID string, status string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop execution repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("sop_id = ? AND customer_id = ? AND status = ?", sopID, customerID, status).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *SopExecutionRepository) GetByID(ctx context.Context, id uint) (*model.SOPExecution, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sop execution repository not initialized")
	}
	var exec model.SOPExecution
	if err := r.db.WithContext(ctx).First(&exec, id).Error; err != nil {
		return nil, err
	}
	return &exec, nil
}

// Save 更新已存在的 SOP 执行记录
//
// 不用 Save：0 行命中会退化成按裸主键 ON CONFLICT，跨租户能把整行搬走/复活。
// 新建走 Create；这里用列 map + WHERE id，0 行即 NotFound。
func (r *SopExecutionRepository) Save(ctx context.Context, exec *model.SOPExecution) error {
	if r == nil || r.db == nil {
		return errors.New("sop execution repository not initialized")
	}
	res := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("id = ?", exec.ID).
		Updates(map[string]any{
			"sop_id":           exec.SOPID,
			"customer_id":      exec.CustomerID,
			"session_id":       exec.SessionID,
			"current_node":     exec.CurrentNode,
			"current_node_idx": exec.CurrentNodeIdx,
			"status":           exec.Status,
			"execution_data":   exec.ExecutionData,
			"started_at":       exec.StartedAt,
			"completed_at":     exec.CompletedAt,
			"error_message":    exec.ErrorMessage,
			"variant":          exec.Variant,
			"last_event_at":    exec.LastEventAt,
			"attempt_count":    exec.AttemptCount,
			"trace_id":         exec.TraceID,
			"wait_event":       exec.WaitEvent,
			"executed_nodes":   exec.ExecutedNodes,
		})
	// 0 行 ⇒ ErrRecordNotFound，统一走 db.EnsureAffected（见 pkg/db/write_guard.go）。
	return _db.EnsureAffected(res)
}

// UpdateAttemptCount 更新执行记录的尝试次数
func (r *SopExecutionRepository) UpdateAttemptCount(ctx context.Context, id uint, attemptCount int) error {
	if r == nil || r.db == nil {
		return errors.New("sop execution repository not initialized")
	}
	return r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("id = ?", id).
		Update("attempt_count", attemptCount).Error
}

// FindStuck 查询卡死的执行记录
// 条件：status=runningStatus 且 started_at < execThreshold 且（updated_at < idleThreshold 或 updated_at IS NULL）
// limit: 最多返回的记录数
func (r *SopExecutionRepository) FindStuck(ctx context.Context, runningStatus string, execThreshold, idleThreshold time.Time, limit int) ([]model.SOPExecution, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	var execs []model.SOPExecution
	err := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("status = ? AND started_at < ? AND (updated_at < ? OR updated_at IS NULL)",
			runningStatus, execThreshold, idleThreshold).
		Order("started_at ASC").
		Limit(limit).
		Find(&execs).Error
	if err != nil {
		return nil, err
	}
	return execs, nil
}

// IncrementSuccessCount 增加指定 SOP 的成功执行计数
// 用于 SOP 执行成功后异步更新 SOPAgent 的 success_count 统计
func (r *SopAgentRepository) IncrementSuccessCount(ctx context.Context, sopID uint) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.SOPAgent{}).
		Where("id = ?", sopID).
		UpdateColumn("success_count", gorm.Expr("success_count + 1")).Error
}

// Create 创建 SOP 智能体
func (r *SopAgentRepository) Create(ctx context.Context, agent *model.SOPAgent) error {
	if r == nil || r.db == nil {
		return errors.New("sop agent repository not initialized")
	}
	return r.db.WithContext(ctx).Create(agent).Error
}

func (r *SopAgentRepository) GetByID(ctx context.Context, id uint) (*model.SOPAgent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sop agent repository not initialized")
	}
	var agent model.SOPAgent
	if err := r.db.WithContext(ctx).First(&agent, id).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// Save 更新已存在的 SOP 智能体（新建走 Create）
//
// 列 map 代替 Save：0 行命中必须报 NotFound，不得按裸主键把行搬到别的租户下。
func (r *SopAgentRepository) Save(ctx context.Context, agent *model.SOPAgent) error {
	if r == nil || r.db == nil {
		return errors.New("sop agent repository not initialized")
	}
	res := r.db.WithContext(ctx).Model(&model.SOPAgent{}).
		Where("id = ?", agent.ID).
		Updates(map[string]any{
			"name":            agent.Name,
			"scenario":        agent.Scenario,
			"description":     agent.Description,
			"trigger_type":    agent.TriggerType,
			"trigger_config":  agent.TriggerConfig,
			"sop_graph":       agent.SOPGraph,
			"version":         agent.Version,
			"is_active":       agent.IsActive,
			"priority":        agent.Priority,
			"execution_count": agent.ExecutionCount,
			"success_count":   agent.SuccessCount,
			"ab_test_config":  agent.ABTestConfig,
			"use_bandit":      agent.UseBandit,
			"created_by":      agent.CreatedBy,
		})
	return _db.EnsureAffected(res)
}

// DeleteByID 按 ID 删除 SOP 智能体，返回受影响行数
func (r *SopAgentRepository) DeleteByID(ctx context.Context, id uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop agent repository not initialized")
	}
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.SOPAgent{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// UpdateActive 按 ID 更新 is_active 字段，返回受影响行数
func (r *SopAgentRepository) UpdateActive(ctx context.Context, id uint, isActive bool) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop agent repository not initialized")
	}
	res := r.db.WithContext(ctx).Model(&model.SOPAgent{}).
		Where("id = ?", id).
		Update("is_active", isActive)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// List 列出 SOP 智能体（scenario 为空时不筛选），分页返回
func (r *SopAgentRepository) List(ctx context.Context, scenario, status string, page, pageSize int) ([]model.SOPAgent, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("sop agent repository not initialized")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	// Count 与 Find 必须各起一条 query（见 base()），不能在同一条链上先 Count 再 Find。
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.SOPAgent{})
		if scenario != "" {
			q = q.Where("scenario = ?", scenario)
		}
		if status != "" {
			switch status {
			case "active":
				q = q.Where("is_active = ?", true)
			default:
				q = q.Where("is_active = ?", false)
			}
		}
		return q
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.SOPAgent
	err := base().Order("priority DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error
	return list, total, err
}

// ListAll 列出所有 SOP 智能体（按 priority DESC, id DESC 排序）
func (r *SopAgentRepository) ListAll(ctx context.Context) ([]model.SOPAgent, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var list []model.SOPAgent
	if err := r.db.WithContext(ctx).Order("priority DESC, id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountAll 统计 SOP 智能体总数
func (r *SopAgentRepository) CountAll(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPAgent{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountActive 统计活跃 SOP 智能体数
func (r *SopAgentRepository) CountActive(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPAgent{}).
		Where("is_active = ?", true).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// IncrementExecutionCount 增加指定 SOP 的执行计数
func (r *SopAgentRepository) IncrementExecutionCount(ctx context.Context, sopID uint) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.SOPAgent{}).
		Where("id = ?", sopID).
		UpdateColumn("execution_count", gorm.Expr("execution_count + 1")).Error
}

// Create 创建 SOP 执行记录
func (r *SopExecutionRepository) Create(ctx context.Context, exec *model.SOPExecution) error {
	if r == nil || r.db == nil {
		return errors.New("sop execution repository not initialized")
	}
	return r.db.WithContext(ctx).Create(exec).Error
}

// UpdateStatus 更新执行记录状态
func (r *SopExecutionRepository) UpdateStatus(ctx context.Context, execID uint, status string) error {
	if r == nil || r.db == nil {
		return errors.New("sop execution repository not initialized")
	}
	return r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("id = ?", execID).
		Update("status", status).Error
}

// UpdateFields 按 ID 更新执行记录的多个字段
func (r *SopExecutionRepository) UpdateFields(ctx context.Context, execID uint, fields map[string]any) error {
	if r == nil || r.db == nil {
		return errors.New("sop execution repository not initialized")
	}
	return r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("id = ?", execID).
		Updates(fields).Error
}

// List 列出 SOP 执行记录（customerID/status 为空时不筛选），分页返回
func (r *SopExecutionRepository) List(ctx context.Context, customerID string, status string, page, pageSize int) ([]model.SOPExecution, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("sop execution repository not initialized")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	// 同 SopAgentRepository.List：Count 与 Find 各起一条 query。
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.SOPExecution{})
		if customerID != "" {
			q = q.Where("customer_id = ?", customerID)
		}
		if status != "" {
			q = q.Where("status = ?", status)
		}
		return q
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.SOPExecution
	err := base().Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error
	return list, total, err
}

// CountByStatus 按 status 统计 SOP 执行数
func (r *SopExecutionRepository) CountByStatus(ctx context.Context, status string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("status = ?", status).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ListBySOPID 列出指定 SOP 的全部执行记录（含 ExecutedNodes，供热力图聚合）
func (r *SopExecutionRepository) ListBySOPID(ctx context.Context, sopID uint, limit int) ([]model.SOPExecution, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sop execution repository not initialized")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var execs []model.SOPExecution
	err := r.db.WithContext(ctx).
		Where("sop_id = ?", sopID).
		Order("created_at DESC").
		Limit(limit).
		Find(&execs).Error
	return execs, err
}

// CountBySOPIDAndVariant 按 SOP ID 和 variant 统计执行数
func (r *SopExecutionRepository) CountBySOPIDAndVariant(ctx context.Context, sopID uint, variant string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop execution repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("sop_id = ? AND variant = ?", sopID, variant).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountBySOPIDAndVariantAndStatus 按 SOP ID、variant 和 status 统计执行数
func (r *SopExecutionRepository) CountBySOPIDAndVariantAndStatus(ctx context.Context, sopID uint, variant string, status string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop execution repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("sop_id = ? AND variant = ? AND status = ?", sopID, variant, status).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountBySOPID 统计指定 SOP 的执行总数（不分 variant / status）
// 用于 FeedbackLearningService 节点转化率分析的端到端转化率分母
func (r *SopExecutionRepository) CountBySOPID(ctx context.Context, sopID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop execution repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("sop_id = ?", sopID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// LatestBySOPAndCustomer 取该客户在某 SOP 下最近一次执行记录；无记录返回 (nil, nil)。
func (r *SopExecutionRepository) LatestBySOPAndCustomer(ctx context.Context, sopID uint, customerID string) (*model.SOPExecution, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sop execution repository not initialized")
	}
	var last model.SOPExecution
	err := r.db.WithContext(ctx).
		Where("sop_id = ? AND customer_id = ?", sopID, customerID).
		Order("created_at DESC").
		First(&last).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &last, nil
}
