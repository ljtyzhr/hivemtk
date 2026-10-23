package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/dbstate"

	"gorm.io/gorm"
)

// TestMessageTraceCleanupRepo_NilDBReturnsError 句柄缺席时必须报错而不是 panic。
//
// 这条用例钉的是一个真实的生产形态：本仓储由 service 包的 init() 构造，那一刻
// db.InitDB 还没跑，`db.GetDB()` 返回 nil。把 nil 存进字段后，清理任务每 24h
// 对 nil 句柄取 WithContext 会 panic，而循环里的 recover 会把 panic 变成一行日志 ——
// 表现为「表正文永远打不掉码、也永远不置 NULL」，不表现为任何报错。
func TestMessageTraceCleanupRepo_NilDBReturnsError(t *testing.T) {
	prev := dbstate.Get()
	dbstate.Set(nil)
	t.Cleanup(func() { dbstate.Set(prev) })

	repo := NewMessageTraceCleanupRepoWithDB(nil)
	ctx := context.Background()

	if _, err := repo.ListForPIIMask(ctx, time.Now(), time.Now().Add(-time.Hour), 0, 10); !errors.Is(err, gorm.ErrInvalidDB) {
		t.Errorf("ListForPIIMask 期望 ErrInvalidDB，实际 %v", err)
	}
	if _, err := repo.UpdateBody(ctx, 1, "a", "b"); !errors.Is(err, gorm.ErrInvalidDB) {
		t.Errorf("UpdateBody 期望 ErrInvalidDB，实际 %v", err)
	}
	if _, err := repo.NullExpiredBodies(ctx, time.Now()); !errors.Is(err, gorm.ErrInvalidDB) {
		t.Errorf("NullExpiredBodies 期望 ErrInvalidDB，实际 %v", err)
	}
}
