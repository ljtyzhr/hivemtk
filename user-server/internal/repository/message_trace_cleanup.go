// message_trace_cleanup.go message_trace 正文 TTL/PII 清理仓储
package repository

import (
	"context"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// MessageTraceCleanupRepo message_trace 清理仓储
type MessageTraceCleanupRepo struct {
	db *gorm.DB
}

// NewMessageTraceCleanupRepo 构造（无参，内部取全局 DB）
func NewMessageTraceCleanupRepo() *MessageTraceCleanupRepo {
	return NewMessageTraceCleanupRepoWithDB(_db.GetDB())
}

// NewMessageTraceCleanupRepoWithDB 显式注入 DB（测试/装配用）
func NewMessageTraceCleanupRepoWithDB(db *gorm.DB) *MessageTraceCleanupRepo {
	return &MessageTraceCleanupRepo{db: db}
}

// conn 返回当前可用的句柄：显式注入的优先，否则每次现取全局句柄。
// 本仓储在 service 包 init() 时构造，那一刻全局 DB 尚未就绪，
// 缓存 nil 句柄会让清理任务每次都 panic 被吞、正文永远清不掉。
func (r *MessageTraceCleanupRepo) conn() (*gorm.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if g := _db.GetDB(); g != nil {
		return g, nil
	}
	return nil, gorm.ErrInvalidDB
}

type TraceMaskRow struct {
	ID     uint
	Input  string
	Output string
}

// ListForPIIMask 取 PII 打码窗口内的一批行（30~90 天区间）
func (r *MessageTraceCleanupRepo) ListForPIIMask(ctx context.Context, upper, lower time.Time, lastID uint, limit int) ([]TraceMaskRow, error) {
	db, err := r.conn()
	if err != nil {
		return nil, err
	}
	var rows []TraceMaskRow
	err = db.WithContext(ctx).
		Table("message_trace").
		Where("created_at <= ? AND created_at > ? AND id > ?", upper, lower, lastID).
		Order("id ASC").Limit(limit).
		Select("id", "input", "output").
		Scan(&rows).Error
	return rows, err
}

func (r *MessageTraceCleanupRepo) UpdateBody(ctx context.Context, id uint, newIn, newOut string) (int64, error) {
	db, err := r.conn()
	if err != nil {
		return 0, err
	}
	res := db.WithContext(ctx).Model(&model.MessageTrace{}).
		Where("id = ?", id).
		Updates(map[string]any{"input": newIn, "output": newOut})
	return res.RowsAffected, res.Error
}

// NullExpiredBodies 将 90 天前的正文置 NULL（结构字段与元数据保留）
func (r *MessageTraceCleanupRepo) NullExpiredBodies(ctx context.Context, cutoff time.Time) (int64, error) {
	db, err := r.conn()
	if err != nil {
		return 0, err
	}
	res := db.WithContext(ctx).Model(&model.MessageTrace{}).
		Where("created_at < ?", cutoff).
		Where("(input IS NOT NULL OR output IS NOT NULL)").
		UpdateColumns(map[string]any{"input": gorm.Expr("NULL"), "output": gorm.Expr("NULL")})
	return res.RowsAffected, res.Error
}
