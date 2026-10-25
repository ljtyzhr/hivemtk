package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/dbencrypt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TenantConfigParamRepository 租户级参数覆盖仓储。
//
// 所有方法都显式带 tenantID 条件：隔离 callback 在 on 模式也会注入同一条件，
// 显式写出来是为了两件事——off 模式（callback 直通）下测试与迁移仍能按租户
// 定位数据；以及任何一次误用（拿了别的租户 ID）都会在 SQL 里看得见。
type TenantConfigParamRepository struct {
	db *gorm.DB
}

// NewTenantConfigParamRepositoryDefault 默认构造（全局句柄；与传 GetDB() 同语义）。
func NewTenantConfigParamRepositoryDefault() *TenantConfigParamRepository {
	return NewTenantConfigParamRepository(GetDB())
}

func NewTenantConfigParamRepository(db *gorm.DB) *TenantConfigParamRepository {
	return &TenantConfigParamRepository{db: db}
}

func (r *TenantConfigParamRepository) Available() bool {
	return r != nil && r.db != nil
}

// ListByGroup 返回某租户在某组的全部覆盖行（读取侧合并用）。
func (r *TenantConfigParamRepository) ListByGroup(ctx context.Context, tenantID uint, group string) ([]model.TenantConfigParam, error) {
	if !r.Available() {
		return nil, ErrConfigParamStoreUnavailable
	}
	var rows []model.TenantConfigParam
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND param_group = ?", tenantID, group).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListAll 返回某租户的全部覆盖行（管理端列表合并用）。
func (r *TenantConfigParamRepository) ListAll(ctx context.Context, tenantID uint) ([]model.TenantConfigParam, error) {
	if !r.Available() {
		return nil, ErrConfigParamStoreUnavailable
	}
	var rows []model.TenantConfigParam
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetByGroupKey 单条覆盖查询；没有覆盖行时返回 gorm.ErrRecordNotFound。
func (r *TenantConfigParamRepository) GetByGroupKey(ctx context.Context, tenantID uint, group, key string) (*model.TenantConfigParam, error) {
	if !r.Available() {
		return nil, ErrConfigParamStoreUnavailable
	}
	var row model.TenantConfigParam
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND param_group = ? AND key = ?", tenantID, group, key).
		First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// UpsertOverride 写入/更新覆盖值并记审计（oldValue 是本次写入前该租户的生效值）。
func (r *TenantConfigParamRepository) UpsertOverride(ctx context.Context, tenantID uint, group, key, newValue, oldValue string, actorID uint) error {
	if !r.Available() {
		return ErrConfigParamStoreUnavailable
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.TenantConfigParam{
			TenantID:  tenantID,
			Group:     group,
			Key:       key,
			Value:     newValue,
			UpdatedBy: actorID,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_id"}, {Name: "param_group"}, {Name: "key"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"param_value", "updated_by", "updated_at"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		encOld, encNew := encryptAuditValues(oldValue, newValue)
		return tx.Create(&model.TenantConfigParamAuditLog{
			TenantID:   tenantID,
			ParamGroup: group,
			ParamKey:   key,
			OldValue:   encOld,
			NewValue:   encNew,
			Action:     "update",
			ActorID:    actorID,
		}).Error
	})
}

// DeleteOverride 删除单条覆盖（重置为该租户的默认值），返回是否真的删掉了行。
func (r *TenantConfigParamRepository) DeleteOverride(ctx context.Context, tenantID uint, group, key, oldValue, defaultValue string, actorID uint) (bool, error) {
	if !r.Available() {
		return false, ErrConfigParamStoreUnavailable
	}
	deleted := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("tenant_id = ? AND param_group = ? AND key = ?", tenantID, group, key).
			Delete(&model.TenantConfigParam{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		deleted = true
		encOld, encNew := encryptAuditValues(oldValue, defaultValue)
		return tx.Create(&model.TenantConfigParamAuditLog{
			TenantID:   tenantID,
			ParamGroup: group,
			ParamKey:   key,
			OldValue:   encOld,
			NewValue:   encNew,
			Action:     "reset",
			ActorID:    actorID,
		}).Error
	})
	return deleted, err
}

// BulkDeleteGroup 清空某租户某组的覆盖行，并按行记审计（新值取平台默认值）。
func (r *TenantConfigParamRepository) BulkDeleteGroup(ctx context.Context, tenantID uint, group string, actorID uint) (int, error) {
	if !r.Available() {
		return 0, ErrConfigParamStoreUnavailable
	}
	var deleted int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var overrides []model.TenantConfigParam
		if err := tx.Where("tenant_id = ? AND param_group = ?", tenantID, group).
			Find(&overrides).Error; err != nil {
			return err
		}
		if len(overrides) == 0 {
			return nil
		}
		defaults := map[string]string{}
		var defs []model.ConfigParam
		if err := tx.Where("param_group = ?", group).Find(&defs).Error; err != nil {
			return err
		}
		for _, d := range defs {
			defaults[d.Key] = d.DefaultValue
		}
		for _, o := range overrides {
			def := defaults[o.Key]
			encOld, encNew := encryptAuditValues(o.Value, def)
			if err := tx.Create(&model.TenantConfigParamAuditLog{
				TenantID:   tenantID,
				ParamGroup: group,
				ParamKey:   o.Key,
				OldValue:   encOld,
				NewValue:   encNew,
				Action:     "bulk_reset",
				ActorID:    actorID,
			}).Error; err != nil {
				return err
			}
			deleted++
		}
		return tx.Where("tenant_id = ? AND param_group = ?", tenantID, group).
			Delete(&model.TenantConfigParam{}).Error
	})
	return deleted, err
}

// AuditLogs 某租户的参数变更日志（输出前解密）。
func (r *TenantConfigParamRepository) AuditLogs(ctx context.Context, tenantID uint, limit int) ([]model.TenantConfigParamAuditLog, error) {
	if !r.Available() {
		return nil, ErrConfigParamStoreUnavailable
	}
	if limit <= 0 {
		limit = 100
	}
	var logs []model.TenantConfigParamAuditLog
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	for i := range logs {
		logs[i].OldValue = dbencrypt.Decrypt(logs[i].OldValue)
		logs[i].NewValue = dbencrypt.Decrypt(logs[i].NewValue)
	}
	return logs, nil
}

// ErrTenantParamContextMissing on 模式下写租户参数却没有租户上下文。
var ErrTenantParamContextMissing = errors.New("多租户模式写参数需要租户上下文（请确认请求经过 TenantMiddleware）")
