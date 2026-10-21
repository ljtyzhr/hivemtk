package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// afterSaleListLimit 售后单查询返回上限：按客户查询不带分页，不封顶会读全表。
const afterSaleListLimit = 200

// AfterSaleRepository 售后单仓库。
// 售后单是客服侧发起、由电商执行落地的记录；本仓库只负责本地持久化与查询。
type AfterSaleRepository struct {
	db *gorm.DB
}

// NewAfterSaleRepositoryWithDB 创建售后单仓库实例（显式注入 db，由装配层传入）。
func NewAfterSaleRepositoryWithDB(db *gorm.DB) *AfterSaleRepository {
	return &AfterSaleRepository{db: db}
}

// Create 创建售后单
func (r *AfterSaleRepository) Create(ctx context.Context, as *model.AfterSale) error {
	return r.db.WithContext(ctx).Create(as).Error
}

// Update 更新售后单（电商回写状态时用）。
// 只写回写会改的字段：Save 会连 tenant_id 与其他列整行覆盖。
func (r *AfterSaleRepository) Update(ctx context.Context, as *model.AfterSale) error {
	res := r.db.WithContext(ctx).Model(&model.AfterSale{}).
		Where("id = ?", as.ID).
		Updates(map[string]any{
			"status":      as.Status,
			"external_id": as.ExternalID,
			"amount":      as.Amount,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListByOrder 按 平台+订单号 列出售后单
func (r *AfterSaleRepository) ListByOrder(ctx context.Context, platform, orderID string) ([]model.AfterSale, error) {
	var list []model.AfterSale
	err := r.db.WithContext(ctx).Where("platform = ? AND order_id = ?", platform, orderID).
		Order("id DESC").Limit(afterSaleListLimit).Find(&list).Error
	return list, err
}

// ListByCustomer 按客户手机列出售后单
func (r *AfterSaleRepository) ListByCustomer(ctx context.Context, phone string) ([]model.AfterSale, error) {
	var list []model.AfterSale
	q := r.db.WithContext(ctx).Model(&model.AfterSale{})
	if phone != "" {
		q = q.Where("customer_phone = ?", phone)
	}
	err := q.Order("id DESC").Limit(afterSaleListLimit).Find(&list).Error
	return list, err
}
