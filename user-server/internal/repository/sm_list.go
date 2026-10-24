package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// errSmlistUnavailable 仓储句柄缺失（装配错，不是「没数据」）。
var errSmlistUnavailable = errors.New("smlist repository not initialized")

// clampSmlistPaging 夹取分页参数：`page = 0` ⇒ offset 为负 ⇒ PG 报
// "OFFSET must not be negative"；`limit = 0` ⇒ GORM 的 Limit(0) 等于**不加 limit**。
func clampSmlistPaging(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	return page, limit
}

type SmlistRepository interface {
	Create(ctx context.Context, user *model.Smlist) error
	GetByID(ctx context.Context, id string) (*model.Smlist, error)
	GetSmlistList(ctx context.Context, page int, limit int) ([]*model.Smlist, int64, error)
	GetSmlistAllList(ctx context.Context) ([]*model.Smlist, int64, error)
	Delete(ctx context.Context, id string) error
	GetRecentSmlistList(ctx context.Context) ([]*model.Smlist, error)
}

type smlistRepo struct {
	db *gorm.DB
}

func NewSmlistRepository(db ...*gorm.DB) SmlistRepository {
	if len(db) > 0 {
		return &smlistRepo{db: db[0]}
	}
	return &smlistRepo{db: _db.GetDB()}
}

func (r *smlistRepo) Create(ctx context.Context, smlist *model.Smlist) error {
	if r == nil || r.db == nil {
		return errSmlistUnavailable
	}
	if smlist == nil {
		return errors.New("smlist: nil 不是写入对象")
	}
	return r.db.WithContext(ctx).Create(smlist).Error
}

// GetByID 按 ID 取短信列表行。
//
// 出错时必须返回 nil 而不是零值行 + 错误：调用方可能选择性忽略 err，
// 若拿到内容为空的记录会当成「存在但字段为空」渲染出去。
func (r *smlistRepo) GetByID(ctx context.Context, id string) (*model.Smlist, error) {
	if r == nil || r.db == nil {
		return nil, errSmlistUnavailable
	}
	var smlist model.Smlist
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&smlist).Error; err != nil {
		return nil, err
	}
	return &smlist, nil
}

func (r *smlistRepo) GetSmlistList(ctx context.Context, page int, limit int) ([]*model.Smlist, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errSmlistUnavailable
	}
	page, limit = clampSmlistPaging(page, limit)

	var smlists []*model.Smlist
	var total int64
	// Count 的错误必须检查：统计失败时 total 是 0，前端会看到「共 0 条」
	// 而列表里却有行，分页器错乱。
	if err := r.db.WithContext(ctx).Model(&model.Smlist{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := r.db.WithContext(ctx).Offset((page - 1) * limit).Limit(limit).Find(&smlists).Error
	return smlists, total, err
}

// GetSmlistAllList 取「全部」短信列表，最多 200 行（超过时调用方需分页重取）。
// query 必须带 ctx：隔离 callback 要从 ctx 读租户绑定，缺 ctx 会被 fail-closed 拒掉。
func (r *smlistRepo) GetSmlistAllList(ctx context.Context) ([]*model.Smlist, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errSmlistUnavailable
	}
	var smlists []*model.Smlist
	var total int64
	if err := r.db.WithContext(ctx).Model(&model.Smlist{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// 无界兜底:默认 200,短信列表表增长后防全表载入;已达上限时调用方需分页重取
	if err := applyListLimit(r.db.WithContext(ctx), 200).Find(&smlists).Error; err != nil {
		return nil, 0, err
	}
	return smlists, total, nil
}

// Delete 按 ID 删除。0 行 ⇒ ErrRecordNotFound。
func (r *smlistRepo) Delete(ctx context.Context, id string) error {
	if r == nil || r.db == nil {
		return errSmlistUnavailable
	}
	return _db.EnsureAffected(r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Smlist{}))
}

// GetRecentSmlistList 取最近 48 小时的短信列表行（上限 200，理由同 GetSmlistAllList）。
func (r *smlistRepo) GetRecentSmlistList(ctx context.Context) ([]*model.Smlist, error) {
	if r == nil || r.db == nil {
		return nil, errSmlistUnavailable
	}
	var smlists []*model.Smlist
	startTime := time.Now().Add(-time.Hour * 48)
	err := applyListLimit(
		r.db.WithContext(ctx).Where("created_at > ?", startTime).Order("created_at desc"),
		200,
	).Find(&smlists).Error
	return smlists, err
}
