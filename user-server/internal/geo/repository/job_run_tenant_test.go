// job_run_tenant_test.go 定时任务运行历史的租户契约。
//
// geo_job_runs 是租户表：历史只含本租户；跨租户回写必须报不存在，
// 不能把「0 行命中」当成回写成功。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	gjrTenantA = uint(321)
	gjrTenantB = uint(322)
)

func gjrSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoJobRun{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoJobRunRepository_CreateAndListTenantScoped(t *testing.T) {
	db := gjrSetupDB(t)
	repo := NewGeoJobRunRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gjrTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gjrTenantB)

	runA := &model.GeoJobRun{JobName: "geo_verify", Trigger: "cron", Status: "running"}
	if err := repo.Create(ctxA, runA); err != nil {
		t.Fatalf("租户 A 写入运行记录失败: %v", err)
	}
	if runA.TenantID != gjrTenantA {
		t.Fatalf("Create 未填租户: %d", runA.TenantID)
	}
	if err := repo.Create(ctxB, &model.GeoJobRun{JobName: "geo_verify", Trigger: "manual", Status: "running"}); err != nil {
		t.Fatalf("租户 B 写入运行记录失败: %v", err)
	}

	listA, totalA, err := repo.List(ctxA, "", 1, 20)
	if err != nil {
		t.Fatalf("租户 A 列表失败: %v", err)
	}
	if totalA != 1 || len(listA) != 1 || listA[0].TenantID != gjrTenantA {
		t.Fatalf("租户 A 历史越界: total=%d list=%+v", totalA, listA)
	}
	if latest, err := repo.Latest(ctxA, "geo_verify"); err != nil || latest.ID != runA.ID {
		t.Fatalf("租户 A Latest 不正确: %+v err=%v", latest, err)
	}
}

func TestGeoJobRunRepository_FinishCrossTenantNotFound(t *testing.T) {
	db := gjrSetupDB(t)
	repo := NewGeoJobRunRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gjrTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gjrTenantB)

	runB := &model.GeoJobRun{JobName: "geo_verify", Trigger: "cron", Status: "running"}
	if err := repo.Create(ctxB, runB); err != nil {
		t.Fatalf("预置租户 B 运行记录失败: %v", err)
	}

	if err := repo.Finish(ctxA, runB.ID, "success", "hijacked", "", 10); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户回写应报不存在，实际 err=%v", err)
	}
	latest, err := repo.Latest(ctxB, "geo_verify")
	if err != nil {
		t.Fatalf("租户 B 回查失败: %v", err)
	}
	if latest.Status != "running" || latest.Summary != "" {
		t.Fatalf("跨租户回写修改了租户 B 的记录: %+v", latest)
	}
}
