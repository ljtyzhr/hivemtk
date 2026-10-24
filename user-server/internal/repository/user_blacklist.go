package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// UserBlacklistRepository 用户黑名单仓储
type UserBlacklistRepository struct {
	db *gorm.DB
}

// NewUserBlacklistRepository 创建黑名单仓储（使用全局 DB）
func NewUserBlacklistRepository() *UserBlacklistRepository {
	return &UserBlacklistRepository{db: _db.GetDB()}
}

// NewUserBlacklistRepositoryWithDB 使用指定 DB 创建仓储（测试用）
func NewUserBlacklistRepositoryWithDB(db *gorm.DB) *UserBlacklistRepository {
	return &UserBlacklistRepository{db: db}
}

// Add 添加黑名单记录
func (r *UserBlacklistRepository) Add(ctx context.Context, b *model.UserBlacklist) error {
	if b == nil {
		return errors.New("blacklist record is nil")
	}
	if b.UserID == "" {
		return errors.New("user_id is required")
	}

	var existing model.UserBlacklist
	err := r.db.WithContext(ctx).Where("user_id = ? AND platform = ? AND active = ?", b.UserID, b.Platform, true).First(&existing).Error
	if err == nil {
		existing.Reason = b.Reason
		existing.Source = b.Source
		existing.OperatorID = b.OperatorID
		existing.OperatorName = b.OperatorName
		existing.SessionID = b.SessionID
		existing.ExpiresAt = b.ExpiresAt
		existing.UpdatedAt = time.Now()
		// 走 db.SaveWholeRow：裸 Save 命中 0 行会回退成 `INSERT ... ON CONFLICT
		// (id) DO UPDATE`（gorm@v1.31.2）。
		//
		// 已知竞态（记录不修）：read-then-First 窗口里并发 Add 同一 (user_id,
		// platform) 会有一方 Create 撞唯一约束（若建了索引）或双写两行；
		// user_blacklist 无 (user_id, platform) 唯一索引（P2 观察），接线幂等
		// 语义时应改 upsert 一条语句。
		return _db.SaveWholeRow(r.db.WithContext(ctx), &existing)
	}
	return r.db.WithContext(ctx).Create(b).Error
}

// IsBlacklisted 判断用户是否在黑名单中（active + 未过期）
func (r *UserBlacklistRepository) IsBlacklisted(ctx context.Context, userID string, platform model.Platform) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var row model.UserBlacklist
	err := r.db.WithContext(ctx).Where("user_id = ? AND platform = ? AND active = ?", userID, platform, true).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if isExpired(&row) {
		_ = r.db.WithContext(ctx).Model(&row).Update("active", false).Error
		return false, nil
	}
	return true, nil
}

// Remove 取消拉黑（软删除：active=false）。0 行 ⇒ ErrRecordNotFound
// （取消一条不存在/已失效的拉黑不该回成功）。
func (r *UserBlacklistRepository) Remove(ctx context.Context, userID string, platform model.Platform) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Model(&model.UserBlacklist{}).
		Where("user_id = ? AND platform = ? AND active = ?", userID, platform, true).
		Update("active", false))
}

// ListActive 查询当前生效的黑名单（active + 未过期）
func (r *UserBlacklistRepository) ListActive(ctx context.Context, page, pageSize int) ([]*model.UserBlacklist, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	var (
		rows  []*model.UserBlacklist
		total int64
	)
	// Count 与 Find 各起一条 query：同一条链上先 Count 再 Find 会就地改
	// Statement（判例 #133 / #150）。
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.UserBlacklist{}).Where("active = ?", true)
	}
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := base().Order("created_at DESC").Offset((page - 1) * pageSize).
		Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func isExpired(b *model.UserBlacklist) bool {
	if b == nil {
		return false
	}
	if b.ExpiresAt == nil {
		return false
	}
	return !b.ExpiresAt.After(time.Now())
}
