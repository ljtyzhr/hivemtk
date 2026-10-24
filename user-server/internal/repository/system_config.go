package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SystemConfigRepository 系统配置仓库接口
type SystemConfigRepository interface {
	GetConfig(ctx context.Context) (*model.SystemConfig, error)
	SaveConfig(ctx context.Context, config *model.SystemConfig) (*model.SystemConfig, error)
	CountUsers(ctx context.Context) (int64, error)
	PingDB(ctx context.Context) bool
}

// ErrSystemConfigStoreUnavailable 表示没装配数据库句柄。
// 与 gorm.ErrRecordNotFound 分开：前者是"没指对库"（装配问题），后者是"库里还没这行"（业务状态）。
// 合成一种的话，运维把上传上限调大不生效时看到的是"没配置过"，真相是"库没指对"，查不到根因。
var ErrSystemConfigStoreUnavailable = errors.New("system_config: 未装配数据库句柄")

type systemConfigRepo struct {
	db *gorm.DB
}

// NewSystemConfigRepository 创建系统配置仓库实例
func NewSystemConfigRepository() SystemConfigRepository {
	return &systemConfigRepo{db: _db.GetDB()}
}

func (r *systemConfigRepo) GetConfig(ctx context.Context) (*model.SystemConfig, error) {
	// 句柄可能没装配（装配顺序不对，或测试里还没 SetDB）。gorm.DB 是 nil 时
	// WithContext 直接空指针崩，而崩在这里等于把"读不到配置"伪装成"进程挂了"。
	// 上传上限这条链路现在每次上传都会走一遍，更不能崩——返回 error 让调用方兜底。
	if r == nil || r.db == nil {
		return nil, ErrSystemConfigStoreUnavailable
	}
	var config model.SystemConfig
	err := r.db.WithContext(ctx).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// SaveConfig 保存系统配置。
// 每租户一行（tenant_id 唯一索引）：已存在（First 命中，租户由隔离 callback
// 限定）时必须原地整行回写（db.SaveWholeRow）——原 FirstOrCreate 在已存在时
// 不写入任何传入字段，前端保存永远是假动作。
//
// TenantID 取自既有行而不是入参：admin 页面回传的 JSON 不带 tenant_id，
// 整行写会连同零值一起 UPDATE，若直接落库就把本租户的配置行写成了 tenant_id=0
// （平台公共池），下一次读取立刻"配置消失"。
func (r *systemConfigRepo) SaveConfig(ctx context.Context, config *model.SystemConfig) (*model.SystemConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrSystemConfigStoreUnavailable
	}
	if config == nil {
		return nil, errors.New("system_config: nil 不是保存对象")
	}
	var existing model.SystemConfig
	err := r.db.WithContext(ctx).First(&existing).Error
	switch {
	case err == nil:
		config.ID = existing.ID
		config.TenantID = existing.TenantID
		// 走 db.SaveWholeRow 而不是裸 Save：上面那条 First 与这条 UPDATE 之间
		// 隔着一次"管理端改配置"的时间窗，行被删掉时裸 Save 会**回退成 INSERT**
		// （gorm@v1.31.2 的 OnConflict{UpdateAll} 分支），把一份带着旧 ID 的
		// 站点配置重新插出来；而 tenant_id 上是唯一索引，front 下一次读取拿到的
		// 到底是哪一行就说不清了。
		if err := _db.SaveWholeRow(r.db.WithContext(ctx), config); err != nil {
			return nil, err
		}
		return config, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		if config.TenantID == 0 {
			config.TenantID = scopeTenantID(ctx)
		}
		if err := r.db.WithContext(ctx).Create(config).Error; err != nil {
			return nil, err
		}
		return config, nil
	default:
		return nil, err
	}
}

func (r *systemConfigRepo) CountUsers(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrSystemConfigStoreUnavailable
	}
	var n int64
	err := r.db.WithContext(ctx).Model(&model.SystemUser{}).Count(&n).Error
	return n, err
}

func (r *systemConfigRepo) PingDB(ctx context.Context) bool {
	if r == nil || r.db == nil {
		return false
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return false
	}
	return sqlDB.PingContext(ctx) == nil
}
