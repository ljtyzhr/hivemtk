package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// ErrDictionaryStoreUnavailable 字典仓储没装配 DB 句柄时的哨兵错。
// 与 config_params / system_config_kv / system_config 三处同一口径。
var ErrDictionaryStoreUnavailable = errors.New("dictionaries: 未装配数据库句柄")

type DictionaryRepository struct {
	db *gorm.DB
}

func NewDictionaryRepository(db *gorm.DB) *DictionaryRepository {
	return &DictionaryRepository{db: db}
}

func (r *DictionaryRepository) available() bool { return r != nil && r.db != nil }

// ListByParent 取一个域下的全部字典项（按 sort_order、code 排序）。
// 不过滤 Enabled：存量行仍可能是停用项，调用方要能把它译成人话。
func (r *DictionaryRepository) ListByParent(ctx context.Context, parent string) ([]model.Dictionary, error) {
	if !r.available() {
		return nil, ErrDictionaryStoreUnavailable
	}
	var out []model.Dictionary
	if err := r.db.WithContext(ctx).
		Where("parent_code = ?", parent).
		Order("sort_order").Order("code").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ListParents 取全部 parent_code（去重排序），供管理端目录用。
func (r *DictionaryRepository) ListParents(ctx context.Context) ([]string, error) {
	if !r.available() {
		return nil, ErrDictionaryStoreUnavailable
	}
	var out []string
	if err := r.db.WithContext(ctx).
		Model(&model.Dictionary{}).
		Distinct().Order("parent_code").
		Pluck("parent_code", &out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *DictionaryRepository) GetByCode(ctx context.Context, parent, code string) (*model.Dictionary, error) {
	if !r.available() {
		return nil, ErrDictionaryStoreUnavailable
	}
	var d model.Dictionary
	err := r.db.WithContext(ctx).
		Where("parent_code = ? AND code = ?", parent, code).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Upsert 建/改一条字典项。已存在则改 Name/Description/SortOrder/Enabled，
// 不动 SystemLocked（那是种子决定的，只能由迁移改）。
func (r *DictionaryRepository) Upsert(ctx context.Context, d *model.Dictionary, actorID uint) error {
	if !r.available() {
		return ErrDictionaryStoreUnavailable
	}
	existing, err := r.GetByCode(ctx, d.ParentCode, d.Code)
	if err != nil {
		return err
	}
	if existing == nil {
		d.UpdatedBy = actorID
		return r.db.WithContext(ctx).Create(d).Error
	}
	existing.Name = d.Name
	existing.Description = d.Description
	existing.SortOrder = d.SortOrder
	existing.Enabled = d.Enabled
	existing.UpdatedBy = actorID
	return r.db.WithContext(ctx).Save(existing).Error
}

// ListTransitions 取一个域的全部跃迁。
func (r *DictionaryRepository) ListTransitions(ctx context.Context, parent string) ([]model.DictionaryTransition, error) {
	if !r.available() {
		return nil, ErrDictionaryStoreUnavailable
	}
	var out []model.DictionaryTransition
	if err := r.db.WithContext(ctx).
		Where("parent_code = ?", parent).
		Order("from_code").Order("to_code").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ReplaceTransitions 用给定集合整体替换一个域的跃迁。
//
// 整体替换而不是逐条增删：跃迁表描述的是"这个域的状态机现在长什么样"，
// 逐条增删会让「管理员以为改完了」和「表里实际的样子」出现中间态——
// 替换是原子的，且调用方传空集合就是清空该域的跃迁（合法语义）。
func (r *DictionaryRepository) ReplaceTransitions(ctx context.Context, parent string, trans []model.DictionaryTransition, actorID uint) error {
	if !r.available() {
		return ErrDictionaryStoreUnavailable
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("parent_code = ?", parent).
			Delete(&model.DictionaryTransition{}).Error; err != nil {
			return err
		}
		for i := range trans {
			trans[i].ParentCode = parent
			trans[i].ID = 0
			trans[i].UpdatedBy = actorID
			if err := tx.Create(&trans[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
