package migration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// 回归钉死 TENANT_MODE=on 时启动期 ExecuteUpgrade 的 fail-closed 缺陷（生产日志实证）：
//
//	[migration] ExecuteUpgrade 启动失败：[tenant] 表 upgrade_tasks 需要租户上下文但 ctx 未绑定（create）
//
// 启动期（cmd/api/main.go）调用点与后台 executeUpgradeAsync goroutine 都没有 HTTP 请求
// ctx 可绑，必须走 tenant.WithSystemScope 这条平台级豁免通道（与 migration/备份/健康检查
// 同类）；同时隔离本身不放宽：普通无租户 ctx 的裸 Create 仍然 fail-closed（下面第一条断言）。
func TestExecuteUpgrade_OnModeWithoutTenantCtx(t *testing.T) {
	database := testutil.NewTestDB(t, &model.UpgradeTask{}, &model.MigrationRecord{})
	if database == nil {
		t.Skip("测试库不可达")
	}
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	// 空注册表：后台 goroutine 无待执行迁移，立即落 completed，避免与测试收尾竞争。
	registry := NewMigrationRegistry()
	svc := NewMigrationService(registry, database, func(*MigrationRegistry, *gorm.DB) {})

	// 隔离闸仍在：无租户 ctx 的裸 Create 必须 fail-closed（本修复不得放宽 HTTP 请求路径）
	if _, err := svc.ExecuteUpgrade(context.Background(), "v1.0.0", "v1.0.0"); err == nil {
		t.Fatal("on 模式无租户 ctx 的 ExecuteUpgrade 必须 fail-closed（隔离不能被本修复放宽）")
	}

	// 启动期（无请求 ctx）走 system scope：Create + 后台 goroutine 落库 + WaitForTask 全链路可用
	sysCtx := tenant.WithSystemScope(context.Background())
	task, err := svc.ExecuteUpgrade(sysCtx, "v1.0.0", "v1.0.0")
	if err != nil {
		t.Fatalf("启动期（无租户 ctx）ExecuteUpgrade 必须在 system scope 下成功: %v", err)
	}
	if task == nil || task.ID == 0 {
		t.Fatal("ExecuteUpgrade 应返回已落库的 upgrade_task")
	}
	// WaitForTask 同样走 system scope：它轮询 upgrade_tasks（租户表），
	// 否则启动期同步等待会 fail-closed 超时。
	if err := svc.WaitForTask(sysCtx, task.ID, 10*time.Second); err != nil {
		t.Fatalf("system scope 下 WaitForTask 应等到后台迁移完成: %v", err)
	}
}

// 装配点锁：单测只证明"传 system scope 的这条路走得通"，但生产真正调用的是
// cmd/api/main.go 的启动期装配点 —— 那里若没包 system scope，单测全绿也锁不住缺陷。
// 静态确认启动期迁移的两处调用点都传了 system-scope ctx。
func TestMainStartupUsesSystemScopeForMigration(t *testing.T) {
	src, err := os.ReadFile("../../cmd/api/main.go")
	if err != nil {
		t.Fatalf("读 cmd/api/main.go 失败: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "migrationCtx := tenant.WithSystemScope(context.Background())") {
		t.Error("main.go 启动期迁移未用 tenant.WithSystemScope 构造 ctx ⇒ on 模式 upgrade_tasks 仍会 fail-closed")
	}
	if !strings.Contains(s, "ExecuteUpgrade(migrationCtx") {
		t.Error("main.go ExecuteUpgrade 调用点未传 migrationCtx（system scope）")
	}
	if !strings.Contains(s, "WaitForTask(migrationCtx") {
		t.Error("main.go WaitForTask 调用点未传 migrationCtx（system scope）")
	}
}
