package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// errSessionTagUnavailable 仓储句柄缺失（无参构造器的仓储必须在方法调用时判空，不能在构造时判）。
var errSessionTagUnavailable = errors.New("session tag repository not initialized")

type SessionTagRepository struct {
	db *gorm.DB
}

func NewSessionTagRepository() *SessionTagRepository {
	return &SessionTagRepository{
		db: _db.GetDB(),
	}
}

func (r *SessionTagRepository) Create(ctx context.Context, tag *model.SessionTag) error {
	if r == nil || r.db == nil {
		return errSessionTagUnavailable
	}
	if tag == nil {
		return errors.New("session tag: nil 不是写入对象")
	}
	return r.db.WithContext(ctx).Create(tag).Error
}

// Update 更新标签（整行回写）；走 db.SaveWholeRow，裸 Save 命中 0 行会回退成 upsert，
// 把句间被删的旧 id 复活。
func (r *SessionTagRepository) Update(ctx context.Context, tag *model.SessionTag) error {
	if r == nil || r.db == nil {
		return errSessionTagUnavailable
	}
	if tag == nil {
		return errors.New("session tag: nil 不是更新对象")
	}
	return _db.SaveWholeRow(r.db.WithContext(ctx), tag)
}

// Delete 删除标签；0 行 ⇒ ErrRecordNotFound。
func (r *SessionTagRepository) Delete(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return errSessionTagUnavailable
	}
	return _db.EnsureAffected(r.db.WithContext(ctx).Delete(&model.SessionTag{}, id))
}

// GetByMerchant 列出当前租户的会话标签（按 sort_order 升序）。
// 行范围由隔离 callback 注入的 tenant_id 条件收窄（on 模式），无需显式过滤。
func (r *SessionTagRepository) GetByMerchant(ctx context.Context) ([]*model.SessionTag, error) {
	if r == nil || r.db == nil {
		return nil, errSessionTagUnavailable
	}
	var tags []*model.SessionTag
	if err := r.db.WithContext(ctx).Order("sort_order ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func (r *SessionTagRepository) GetByID(ctx context.Context, id uint) (*model.SessionTag, error) {
	if r == nil || r.db == nil {
		return nil, errSessionTagUnavailable
	}
	var tag model.SessionTag
	if err := r.db.WithContext(ctx).First(&tag, id).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

// UpdateRuleCondition 更新标签规则条件（按 code 定位；空串=清除规则仅手动）
func (r *SessionTagRepository) UpdateRuleCondition(ctx context.Context, code, condition string) error {
	if r == nil || r.db == nil {
		return errSessionTagUnavailable
	}
	// 0 行 ⇒ 没有这个 code 的标签 ⇒ ErrRecordNotFound（经 db.EnsureAffected）。
	return _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.SessionTag{}).
		Where("code = ?", code).
		Update("rule_condition", condition))
}
