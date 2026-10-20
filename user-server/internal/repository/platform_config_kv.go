package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// PlatformConfigKVRepository 平台级 KV 配置仓储接口（实例级，无租户分域）。
//
// 键空间见 model.PlatformConfigKV 注释：桥接/MCP/回调验签/宿主令牌与平台
// 安全、备份策略。这些键在登录前或非租户入口被读取，必须与租户上下文解耦。
type PlatformConfigKVRepository interface {
	Available() bool
	Get(ctx context.Context, key string) (string, error)
	Upsert(ctx context.Context, key, value string) (string, error)
	EnsureTable(ctx context.Context) error
}

type platformConfigKVRepo struct {
	db *gorm.DB
}

// NewPlatformConfigKVRepository 构造（捕获调用那一刻的全局句柄，语义同 KV 仓储）。
func NewPlatformConfigKVRepository() PlatformConfigKVRepository {
	return &platformConfigKVRepo{db: db.GetDB()}
}

// NewPlatformConfigKVRepositoryWithDB 用显式句柄构造（装配点句柄一致性，同 WithDB 口径）。
func NewPlatformConfigKVRepositoryWithDB(gormDB *gorm.DB) PlatformConfigKVRepository {
	return &platformConfigKVRepo{db: gormDB}
}

func (r *platformConfigKVRepo) Available() bool {
	return r != nil && r.db != nil
}

func (r *platformConfigKVRepo) Get(ctx context.Context, key string) (string, error) {
	if !r.Available() {
		return "", ErrKVStoreUnavailable
	}
	var row model.PlatformConfigKV
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return row.Value, nil
}

func (r *platformConfigKVRepo) Upsert(ctx context.Context, key, value string) (string, error) {
	if !r.Available() {
		return "", ErrKVStoreUnavailable
	}
	now := time.Now()
	row := model.PlatformConfigKV{
		Key:       key,
		Value:     value,
		CreatedAt: now,
		UpdatedAt: now,
	}
	upsert := func() error {
		return r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "key"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"value":      value,
				"updated_at": now,
			}),
		}).Create(&row).Error
	}
	err := upsert()
	if err != nil {
		if ensureErr := r.EnsureTable(ctx); ensureErr != nil {
			return "", err
		}
		err = upsert()
		if err != nil {
			return "", err
		}
	}
	return value, nil
}

func (r *platformConfigKVRepo) EnsureTable(ctx context.Context) error {
	if !r.Available() {
		return ErrKVStoreUnavailable
	}
	return r.db.WithContext(ctx).AutoMigrate(&model.PlatformConfigKV{})
}

var _ PlatformConfigKVRepository = (*platformConfigKVRepo)(nil)
