package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/content/model"

	"gorm.io/gorm"
)

// ErrMarketingFlowMissing 营销流程在本租户内不存在。
//
// 消息必须带「不存在」：出口层按消息判定 404，换成别的措辞就变成 500。
var ErrMarketingFlowMissing = errors.New("营销流程不存在")

// MarketingFlowRepository 营销流程仓库
type MarketingFlowRepository struct {
	db *gorm.DB
}

// NewMarketingFlowRepository 创建营销流程仓库（句柄由装配处交进来）
func NewMarketingFlowRepository(db *gorm.DB) *MarketingFlowRepository {
	return &MarketingFlowRepository{
		db: db,
	}
}

// Create 创建流程
func (r *MarketingFlowRepository) Create(ctx context.Context, flow *model.MarketingFlow) error {
	return r.db.WithContext(ctx).Create(flow).Error
}

// GetByID 根据 ID 获取流程。
//
// 失败一律交出 (nil, err)：命中不到既可能是记录不存在，也可能是调用方没带租户上下文，
// 回一个零值结构体等于让上层无从分辨，只能把配置级故障说成「流程不存在」。
func (r *MarketingFlowRepository) GetByID(ctx context.Context, id uint) (*model.MarketingFlow, error) {
	var flow model.MarketingFlow
	if err := r.db.WithContext(ctx).First(&flow, id).Error; err != nil {
		return nil, err
	}
	return &flow, nil
}

// GetAll 获取本租户的流程列表
func (r *MarketingFlowRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.MarketingFlow, int64, error) {
	page, pageSize = normalizeFlowPaging(page, pageSize)

	var flows []*model.MarketingFlow
	var total int64

	cond := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.MarketingFlow{})
	}
	if err := cond().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := cond().
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&flows).Error
	if err != nil {
		return nil, 0, err
	}
	return flows, total, nil
}

// Update 全字段覆盖本租户的既有流程。
//
// 不用 Save：Save 走的是 INSERT ... ON CONFLICT DO UPDATE（upsert），主键命中不到行时
// 会把**整行按结构体里的 tenant_id 重新插入**——读改写之间流程被别处删掉，一次「更新」
// 就成了跨租户复活。改成按主键 + 租户闸的 UPDATE，命中 0 行如实交出缺失错误。
func (r *MarketingFlowRepository) Update(ctx context.Context, flow *model.MarketingFlow) error {
	res := r.db.WithContext(ctx).Model(flow).
		Select("*").Omit("id", "created_at", "tenant_id").Updates(flow)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMarketingFlowMissing
	}
	return nil
}

// Delete 删除流程
func (r *MarketingFlowRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.MarketingFlow{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMarketingFlowMissing
	}
	return nil
}

// UpdateStatus 更新流程状态：命中 0 行（流程不存在或不属于本租户）交出缺失错误。
func (r *MarketingFlowRepository) UpdateStatus(ctx context.Context, id uint, status model.FlowStatus) error {
	res := r.db.WithContext(ctx).Model(&model.MarketingFlow{}).Where("id = ?", id).Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMarketingFlowMissing
	}
	return nil
}

// FlowExecutionRepository 流程执行仓库
type FlowExecutionRepository struct {
	db *gorm.DB
}

// NewFlowExecutionRepository 创建流程执行仓库（句柄由装配处交进来）
func NewFlowExecutionRepository(db *gorm.DB) *FlowExecutionRepository {
	return &FlowExecutionRepository{
		db: db,
	}
}

// Create 创建执行记录
func (r *FlowExecutionRepository) Create(ctx context.Context, execution *model.FlowExecution) error {
	return r.db.WithContext(ctx).Create(execution).Error
}

// GetByFlowID 根据流程 ID 分页获取本租户的执行记录
func (r *FlowExecutionRepository) GetByFlowID(ctx context.Context, flowID uint, page, pageSize int) ([]*model.FlowExecution, int64, error) {
	page, pageSize = normalizeFlowPaging(page, pageSize)

	var executions []*model.FlowExecution
	var total int64

	cond := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.FlowExecution{}).Where("flow_id = ?", flowID)
	}
	if err := cond().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := cond().
		Order("started_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&executions).Error
	if err != nil {
		return nil, 0, err
	}
	return executions, total, nil
}

// Update 更新执行记录
func (r *FlowExecutionRepository) Update(ctx context.Context, execution *model.FlowExecution) error {
	return r.db.WithContext(ctx).Save(execution).Error
}

// GetStats 按状态聚合本租户某流程的执行统计
func (r *FlowExecutionRepository) GetStats(ctx context.Context, flowID uint) (map[string]int64, error) {
	var stats []struct {
		Status string `gorm:"column:status"`
		Count  int64  `gorm:"column:count"`
	}

	if err := r.db.WithContext(ctx).Model(&model.FlowExecution{}).
		Select("status, COUNT(*) as count").
		Where("flow_id = ?", flowID).
		Group("status").
		Scan(&stats).Error; err != nil {
		return nil, err
	}

	result := make(map[string]int64, len(stats))
	for _, s := range stats {
		result[s.Status] = s.Count
	}
	return result, nil
}

// normalizeFlowPaging 夹紧分页参数：page=0 会算出负 OFFSET，PG 直接报错，
// 一个越界的 page_size 则能把整表拖进一次响应。
func normalizeFlowPaging(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}
