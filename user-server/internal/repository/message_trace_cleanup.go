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

// NewMessageTraceCleanupRepo 构造（无参，内部取全局 DB；对齐 customer_session.go 先例）
func NewMessageTraceCleanupRepo() *MessageTraceCleanupRepo {
	return NewMessageTraceCleanupRepoWithDB(_db.GetDB())
}

// NewMessageTraceCleanupRepoWithDB 显式注入 DB（测试/装配用）
func NewMessageTraceCleanupRepoWithDB(db *gorm.DB) *MessageTraceCleanupRepo {
	return &MessageTraceCleanupRepo{db: db}
}

// conn 返回当前可用的句柄：显式注入的优先，否则**每次现取**全局句柄。
//
// 本仓储由 service 包的 init() 构造，那一刻 db.InitDB 还没跑，`_db.GetDB()`
// 必然是 nil —— 把 nil 存进字段就意味着清理任务此后每次都对 nil 句柄取 WithContext
// 而 panic（被循环里的 recover 吞掉），表正文永远打不掉码、也永远不置 NULL。
func (r *MessageTraceCleanupRepo) conn() (*gorm.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if g := _db.GetDB(); g != nil {
		return g, nil
	}
	return nil, gorm.ErrInvalidDB
}

// TraceMaskRow PII 打码批次行
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

// UpdateBody 更新单行 input/output
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
