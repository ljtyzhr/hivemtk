package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// MemoryDecayRepository L-4 记忆衰减归档的读写口。
//
// 租户条件由 ctx 承载，本仓储**不**收显式 tenantID：调用方一律经
// `tenant.Fanout` 进入，每次回调的 ctx 都绑定了唯一租户，SaaS 隔离 callback 会
// 把 tenant_id 条件注入这里每一条语句；off 模式 Fanout 给的是默认租户 ctx。
// 也就是说这条链上没有 SystemScope 通道，也就不存在"绕过隔离"的口子——
// 这比在每条 Where 里手写 tenant_id 更稳（手写的那一半不会随 callback 变化同步）。
type MemoryDecayRepository struct {
	db *gorm.DB
}

func NewMemoryDecayRepository(db *gorm.DB) *MemoryDecayRepository {
	return &MemoryDecayRepository{db: db}
}

// ListActiveLongTermItems 取当前租户下未失效的 L2 长期记忆事实行（单轮上限 limit）。
func (r *MemoryDecayRepository) ListActiveLongTermItems(ctx context.Context, limit int) ([]model.MemoryItem, error) {
	var items []model.MemoryItem
	err := r.db.WithContext(ctx).
		Where("layer = ? AND invalid_at IS NULL", model.MemoryLayerLongTerm).
		Limit(limit).
		Find(&items).Error
	return items, err
}

// ListActiveLongTermMemory 取当前租户下未失效的客户长期记忆行（单轮上限 limit）。
func (r *MemoryDecayRepository) ListActiveLongTermMemory(ctx context.Context, limit int) ([]model.CustomerLongTermMemory, error) {
	var rows []model.CustomerLongTermMemory
	err := r.db.WithContext(ctx).
		Where("invalid_at IS NULL").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// MarkItemsInvalid 软失效一批事实行（写 invalid_at，不物理删）。
func (r *MemoryDecayRepository) MarkItemsInvalid(ctx context.Context, ids []uint, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MemoryItem{}).
		Where("id IN ?", ids).
		Update("invalid_at", at).Error
}

// MarkLongTermMemoryInvalid 软失效一批客户长期记忆行（写 invalid_at，不物理删）。
func (r *MemoryDecayRepository) MarkLongTermMemoryInvalid(ctx context.Context, ids []uint64, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.CustomerLongTermMemory{}).
		Where("id IN ?", ids).
		Update("invalid_at", at).Error
}
