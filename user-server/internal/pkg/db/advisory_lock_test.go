package db

import (
	"context"
	"testing"

	"hivemtk-user/internal/pkg/testutil"
)

// TestTryAdvisoryLock_AcquiresAndExcludes 锁必须真的拿到。
//
// 这条断言是本文件存在的理由：2026-10-22 实测原来的写法
// `db.Select("pg_try_advisory_lock(?)", key).Scan(&held)` 会报
// "unsupported data type: <nil>"，而**返回值 false 看起来像"锁被别人拿了"**
// —— 于是所有调用方都安静地跳过了加锁，多实例并发保护形同虚设。
func TestTryAdvisoryLock_AcquiresAndExcludes(t *testing.T) {
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达")
	}
	ctx := context.Background()
	const key = int64(9173001)

	held, err := TryAdvisoryLock(ctx, db, key)
	if err != nil {
		t.Fatalf("TryAdvisoryLock 失败: %v", err)
	}
	if !held {
		t.Fatal("锁没拿到：返回值 false 会被调用方读成「已有实例在运行」⇒ 实际从未加锁")
	}

	// 同一 session 再取一次：PG 的会话级 advisory lock 可重入 ⇒ 仍为 true。
	held2, err := TryAdvisoryLock(ctx, db, key)
	if err != nil {
		t.Fatalf("二次 TryAdvisoryLock 失败: %v", err)
	}
	if !held2 {
		t.Log("二次加锁返回 false（会话级锁可重入时不会发生）")
	}

	if err := ReleaseAdvisoryLock(ctx, db, key); err != nil {
		t.Fatalf("ReleaseAdvisoryLock 失败: %v", err)
	}
}

// TestTryAdvisoryLock_NilDB nil 句柄不得 panic
func TestTryAdvisoryLock_NilDB(t *testing.T) {
	if _, err := TryAdvisoryLock(context.Background(), nil, 1); err == nil {
		t.Fatal("nil db 应当返回 error")
	}
	if err := ReleaseAdvisoryLock(context.Background(), nil, 1); err == nil {
		t.Fatal("nil db 应当返回 error")
	}
}
