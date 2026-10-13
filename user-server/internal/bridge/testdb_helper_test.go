package bridge

import (
	"testing"

	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupBridgeTestDB 建进程级测试库并注入全局。
// NewInboxIngressService 无参构造在内部经 dbUtil.GetDB() 取库，
// 本包测试若不 SetTestDB，仓库会拿到 nil DB 并在查询时 panic。
func setupBridgeTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t, models...)
	db.SetTestDB(database)
	t.Cleanup(func() { db.SetTestDB(nil) })
	return database
}
