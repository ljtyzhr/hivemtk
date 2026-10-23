package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/dbencrypt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 变更审计里的改前/改后值默认加密落库（OPT-SEC-04 残项，与 api_logs 同一把钥匙）。
//
// 为什么审计表也要加密：config_params 里存着 API Key 一类的敏感参数（见
// config_param_group_key_unique 的复合唯一键分组），明文留痕等于把当前与历史
// 密钥全量抄进一张无访问控制的表 —— 比主表泄露更糟，主表至少有 config 读取
// 权限，审计表通常是「谁改了什么」的运维只读视图。
//
// 列类型已是 text，无需迁移（对照 api_logs 需要 057 迁移纯粹是因为
// ip_address 是 varchar(45) 装不下密文）。存量明文行不做批量改写：AES-256-GCM
// 的 nonce 随机生成且密钥只在应用侧，纯 SQL 改不了；读取出口 Decrypt 对非
// `enc:v1:` 前缀原样返回，故新旧混读安全。
func encryptAuditValues(oldValue, newValue string) (string, string) {
	return dbencrypt.Encrypt(oldValue), dbencrypt.Encrypt(newValue)
}

// ErrConfigParamStoreUnavailable 参数中心仓储没装配 DB 句柄时的哨兵错。
//
// 与 0.4 的 ErrKVStoreUnavailable / 0.5 的 ErrSystemConfigStoreUnavailable 同一口径：
// 返回 error 而不是让 gorm 的 nil DB 在 WithContext 上 panic。装配顺序出错
// （setupConfigParamRoutes 早于 SeedConfigParams、或 NewConfigParamService(nil)）时
// 这条路径必然被走到，panic 会把一次配置面故障放大成整站起不来。
var ErrConfigParamStoreUnavailable = errors.New("config_params: 未装配数据库句柄")

// ConfigParamRepository 动态参数仓储
type ConfigParamRepository struct {
	db *gorm.DB
}

func NewConfigParamRepository(db *gorm.DB) *ConfigParamRepository {
	return &ConfigParamRepository{db: db}
}

// List 返回参数（管理端用）。
//
// 默认**不含**已下架的旋钮。理由：页面上挂着一个改了不生效的旋钮，比没有这个
// 旋钮更糟——用户改完排查半天，问题却出在代码里而不是配置里。需要查历史值时用
// includeDeprecated=true（或 API 的 ?include_deprecated=1）。
func (r *ConfigParamRepository) List(ctx context.Context, includeDeprecated bool) ([]model.ConfigParam, error) {
	if r == nil || r.db == nil {
		return nil, ErrConfigParamStoreUnavailable
	}
	q := r.db.WithContext(ctx)
	if !includeDeprecated {
		q = q.Where("deprecated = ?", false)
	}
	var params []model.ConfigParam
	if err := q.Order("param_group").Order("key").Find(&params).Error; err != nil {
		return nil, err
	}
	return params, nil
}

// ListByGroup 按分组返回参数（管理端分组展示），口径与 List 一致。
func (r *ConfigParamRepository) ListByGroup(ctx context.Context, group string, includeDeprecated bool) ([]model.ConfigParam, error) {
	if r == nil || r.db == nil {
		return nil, ErrConfigParamStoreUnavailable
	}
	q := r.db.WithContext(ctx).Where(&model.ConfigParam{Group: group})
	if !includeDeprecated {
		q = q.Where("deprecated = ?", false)
	}
	var params []model.ConfigParam
	if err := q.Order("key").Find(&params).Error; err != nil {
		return nil, err
	}
	return params, nil
}

// UpsertGroups 写入分组元数据（seed 期调用）。
//
// 用 upsert 而不是先删后插：分组表没有历史可追溯需求，但它可能被运营手工排过序，
// 启动时整表重建会把运营的排序抹掉。逐组按 group 唯一键更新才是「代码是默认值、
// 运营可覆盖」的正确形态。
func (r *ConfigParamRepository) UpsertGroups(ctx context.Context, groups []model.ConfigParamGroup) error {
	if r == nil || r.db == nil {
		return ErrConfigParamStoreUnavailable
	}
	if len(groups) == 0 {
		return nil
	}
	rows := make([]model.ConfigParamGroup, len(groups))
	copy(rows, groups)
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "param_group"}},
			DoUpdates: clause.AssignmentColumns([]string{"label", "description", "sort_order", "updated_at"}),
		}).
		Create(&rows).Error
}

// ListGroups 返回全部分组元数据，按 sort_order、group 排序。
//
// sort_order=0 是「没排过序」的哨兵值，它必须排**最后**而不是最前。
// 直接 `ORDER BY sort_order` 会让 0 顶到最前，于是每加一个没配顺序的新组，
// 管理端侧栏的顺序就在两次发布之间悄悄变 —— 而顺序是组级属性，运维是照着
// 记忆在侧栏里找组的，顺序一飘就找不到。`(sort_order = 0)` 在 Postgres 里
// 布尔序 false < true，天然把"排过序的"排在"没排的"前面。
func (r *ConfigParamRepository) ListGroups(ctx context.Context) ([]model.ConfigParamGroup, error) {
	if r == nil || r.db == nil {
		return nil, ErrConfigParamStoreUnavailable
	}
	var groups []model.ConfigParamGroup
	if err := r.db.WithContext(ctx).
		Order("(sort_order = 0)").
		Order("sort_order").
		Order("param_group").
		Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

// GetByGroupKey 单条查询（Service 层类型化读取的底层）
func (r *ConfigParamRepository) GetByGroupKey(ctx context.Context, group, key string) (*model.ConfigParam, error) {
	if r == nil || r.db == nil {
		return nil, ErrConfigParamStoreUnavailable
	}
	var p model.ConfigParam
	if err := r.db.WithContext(ctx).Where(&model.ConfigParam{Group: group, Key: key}).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ErrParamDeprecated 试图改动一个已下架的旋钮。
//
// 为什么写入要拦：下架的语义是「这个旋钮改了不生效」。如果 API 还收下改动、
// 审计里还留下一条 update 记录，用户看到「保存成功 + 有审计」却发现行为没变——
// 这比直接报错糟糕得多，它把一个明确的失败变成了一个需要人去推理的矛盾。
var ErrParamDeprecated = errors.New("该参数已下架（改了也不生效），如需恢复请先在代码里重新接线")

// ErrParamReadOnly 试图改动一个只读参数（由系统推导或锁死，比如嵌入维度）。
//
// 早先返回的是 gorm.ErrRecordNotFound：管理端列表里那一行明明白白存在，
// 点保存却被告知「记录不存在」，读起来像是页面或索引坏了。哨兵必须说清真正的原因。
var ErrParamReadOnly = errors.New("该参数为只读，不可在管理端修改（要变更请走代码与迁移）")

// UpdateValue 更新参数值（含 actor 审计）
func (r *ConfigParamRepository) UpdateValue(ctx context.Context, group, key, newValue string, actorID uint) error {
	if r == nil || r.db == nil {
		return ErrConfigParamStoreUnavailable
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.ConfigParam
		if err := tx.Where(&model.ConfigParam{Group: group, Key: key}).First(&p).Error; err != nil {
			return err
		}
		if p.ReadOnly {
			return ErrParamReadOnly
		}
		if p.Deprecated {
			return ErrParamDeprecated
		}
		oldValue := p.Value
		updates := map[string]any{
			"Value":      newValue,
			"updated_by": actorID,
		}
		if err := tx.Model(&p).Updates(updates).Error; err != nil {
			return err
		}
		encOld, encNew := encryptAuditValues(oldValue, newValue)
		return tx.Create(&model.ConfigParamAuditLog{
			ParamGroup: group,
			ParamKey:   key,
			OldValue:   encOld,
			NewValue:   encNew,
			Action:     "update",
			ActorID:    actorID,
		}).Error
	})
}

// TableReady 启动 seed 的前置自检：表能读就打住（空表也算就绪）。
//
// 放仓储而不是 service 里就地 `db.WithContext(...)`：五层边界要求 service/controller
// 不持有 *gorm.DB 自行发查询（同 kb_item_binding 审计 A14 的口径），否则「这条链用了哪
// 几张表」只在 grep 过全文件之后才知道。
func (r *ConfigParamRepository) TableReady(ctx context.Context) error {
	if r == nil || r.db == nil {
		return ErrConfigParamStoreUnavailable
	}
	if err := r.db.WithContext(ctx).First(&model.ConfigParam{}).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

// Create 落一条参数行（启动 seed 补齐缺项用）。
func (r *ConfigParamRepository) Create(ctx context.Context, p *model.ConfigParam) error {
	if r == nil || r.db == nil {
		return ErrConfigParamStoreUnavailable
	}
	return r.db.WithContext(ctx).Create(p).Error
}

// ApplyDefinitionUpdates 按主键把「定义列」刷成当前种子定义（不碰 value）。
//
// updates 的键是 model 字段名（GORM Updates 的既有口径），由 service 侧算 diff：
// 「哪些列算定义、哪些改动该放过」是领域策略，落在 service；「怎么写到库里」落在这里。
func (r *ConfigParamRepository) ApplyDefinitionUpdates(ctx context.Context, id uint, updates map[string]any) error {
	if r == nil || r.db == nil {
		return ErrConfigParamStoreUnavailable
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.ConfigParam{}).
		Where("id = ?", id).Updates(updates).Error
}

// PruneRetiredGroups 清掉"已从种子摘除、且组内再无在用参数"的分组行。
//
// keep 是当前种子的组名全集。两条必须同时成立才删：
//   - 组名不在 keep 里（种子已经不声明它）
//   - 该组没有 deprecated=false 的参数（还有活旋钮 ⇒ 删了等于把活旋钮的入口也藏掉）
//
// 用 NOT EXISTS 而不是先 SELECT 再 DELETE：并发 seed（多实例同启）下"查完才发现有活参数"
// 的窗口里刚好有人加了参数的话，先查后删会误删。
func (r *ConfigParamRepository) PruneRetiredGroups(ctx context.Context, keep []string) (int, error) {
	if r == nil || r.db == nil {
		return 0, ErrConfigParamStoreUnavailable
	}
	if len(keep) == 0 {
		// 拿不到种子全集时一律不动：把"没读到"当成"全部下架"会清空整张分组表。
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("param_group NOT IN ?", keep).
		Where(`NOT EXISTS (SELECT 1 FROM config_params p
		         WHERE p.param_group = config_param_groups.param_group
		           AND p.deprecated = ?)`, false).
		Delete(&model.ConfigParamGroup{})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

// ResetToDefault 重置单条为默认值
//
// 只读与下架都要拦：整组重置本来就跳过只读行，单条却不判，
// 等于留了一条「批量拦得住、逐条能绕过」的旁路。
func (r *ConfigParamRepository) ResetToDefault(ctx context.Context, group, key string, actorID uint) error {
	if r == nil || r.db == nil {
		return ErrConfigParamStoreUnavailable
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.ConfigParam
		if err := tx.Where(&model.ConfigParam{Group: group, Key: key}).First(&p).Error; err != nil {
			return err
		}
		if p.ReadOnly {
			return ErrParamReadOnly
		}
		if p.Deprecated {
			return ErrParamDeprecated
		}
		oldValue := p.Value
		if err := tx.Model(&p).Updates(map[string]any{
			"Value":      p.DefaultValue,
			"updated_by": actorID,
		}).Error; err != nil {
			return err
		}
		encOld, encNew := encryptAuditValues(oldValue, p.DefaultValue)
		return tx.Create(&model.ConfigParamAuditLog{
			ParamGroup: group,
			ParamKey:   key,
			OldValue:   encOld,
			NewValue:   encNew,
			Action:     "reset",
			ActorID:    actorID,
		}).Error
	})
}

// BulkResetGroup 整组重置默认值
func (r *ConfigParamRepository) BulkResetGroup(ctx context.Context, group string, actorID uint) error {
	if r == nil || r.db == nil {
		return ErrConfigParamStoreUnavailable
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var params []model.ConfigParam
		if err := tx.Where(&model.ConfigParam{Group: group}).Find(&params).Error; err != nil {
			return err
		}
		for _, p := range params {
			if p.ReadOnly || p.Deprecated || p.Value == p.DefaultValue {
				continue
			}
			oldValue := p.Value
			if err := tx.Model(&p).Update("Value", p.DefaultValue).Error; err != nil {
				return err
			}
			encOld, encNew := encryptAuditValues(oldValue, p.DefaultValue)
			if err := tx.Create(&model.ConfigParamAuditLog{
				ParamGroup: p.Group,
				ParamKey:   p.Key,
				OldValue:   encOld,
				NewValue:   encNew,
				Action:     "bulk_reset",
				ActorID:    actorID,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// AuditLogs 变更日志查询（管理端只读）
func (r *ConfigParamRepository) AuditLogs(ctx context.Context, limit int) ([]model.ConfigParamAuditLog, error) {
	if r == nil || r.db == nil {
		return nil, ErrConfigParamStoreUnavailable
	}
	if limit <= 0 {
		limit = 100
	}
	var logs []model.ConfigParamAuditLog
	if err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, err
	}
	for i := range logs {
		logs[i].OldValue = dbencrypt.Decrypt(logs[i].OldValue)
		logs[i].NewValue = dbencrypt.Decrypt(logs[i].NewValue)
	}
	return logs, nil
}
