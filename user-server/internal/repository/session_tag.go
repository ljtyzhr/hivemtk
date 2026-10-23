package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// errSessionTagUnavailable 仓储句柄缺失（`init()` 时刻 `_db.GetDB()` 必为 nil，
// 见 tracker #44 的系统性形状：无参构造器的仓储必须在方法调用时判空，不能在构造时判）。
var errSessionTagUnavailable = errors.New("session tag repository not initialized")

// SessionTagRepository 会话标签仓库
type SessionTagRepository struct {
	db *gorm.DB
}

// NewSessionTagRepository 创建会话标签仓库实例
func NewSessionTagRepository() *SessionTagRepository {
	return &SessionTagRepository{
		db: _db.GetDB(),
	}
}

// Create 创建标签
func (r *SessionTagRepository) Create(ctx context.Context, tag *model.SessionTag) error {
	if r == nil || r.db == nil {
		return errSessionTagUnavailable
	}
	if tag == nil {
		return errors.New("session tag: nil 不是写入对象")
	}
	return r.db.WithContext(ctx).Create(tag).Error
}

// Update 更新标签（整行回写）。
//
// 走 `db.SaveWholeRow`：裸 Save 命中 0 行会回退成 `INSERT ... ON CONFLICT (id)
// DO UPDATE`，把「service 读出之后、写回之前被删掉的那条」按原 id 插回去 ——
// 一条没人建过的标签重新出现在所有会话的标签下拉里。
func (r *SessionTagRepository) Update(ctx context.Context, tag *model.SessionTag) error {
	if r == nil || r.db == nil {
		return errSessionTagUnavailable
	}
	if tag == nil {
		return errors.New("session tag: nil 不是更新对象")
	}
	return _db.SaveWholeRow(r.db.WithContext(ctx), tag)
}

// Delete 删除标签。
//
// 0 行 ⇒ ErrRecordNotFound：删一条已经不存在的标签不该回成功。
func (r *SessionTagRepository) Delete(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return errSessionTagUnavailable
	}
	return _db.EnsureAffected(r.db.WithContext(ctx).Delete(&model.SessionTag{}, id))
}

// GetByMerchant 列出当前租户的会话标签（按 sort_order 升序）。
//
// 原先的条件是 `Where("1 = 1")`：一个恒真占位符，看起来像「这里本该有个条件」。
// 实际收窄靠隔离 callback 注入的 `tenant_id = ?`（on 模式），而恒真条件什么也
// 没约束 —— 它唯一的作用是让下一个读代码的人以为筛选逻辑在别处。
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

// GetByID 根据ID获取标签
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
	// 0 行 ⇒ 没有这个 code 的标签 ⇒ ErrRecordNotFound。手写的
	// `RowsAffected == 0 → gorm.ErrRecordNotFound` 与 `db.EnsureAffected` 是同一件事，
	// 统一走后者：判据只有一处，改的时候不会漏掉某一支。
	return _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.SessionTag{}).
		Where("code = ?", code).
		Update("rule_condition", condition))
}
