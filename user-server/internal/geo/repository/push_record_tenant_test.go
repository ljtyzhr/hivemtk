// push_record_tenant_test.go 蜘蛛推送记录的租户契约。
//
// geo_push_records 是租户表：批量落库逐行填 tenant_id；
// on 模式下 ctx 未绑租户必须 fail-closed，不能落进默认租户。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	gprcTenantA = uint(363)
	gprcTenantB = uint(364)
)

func gprcSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoPushRecord{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoPushRecordRepository_TenantScoped(t *testing.T) {
	db := gprcSetupDB(t)
	repo := NewGeoPushRecordRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gprcTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gprcTenantB)
	sysCtx := tenant.WithSystemScope(context.Background())

	batchA := []*model.GeoPushRecord{
		{Platform: "baidu", URL: "https://a.example/1", BatchID: "batch-a", SuccessCount: 1},
		{Platform: "baidu", URL: "https://a.example/2", BatchID: "batch-a", FailCount: 1},
	}
	if err := repo.BatchCreate(ctxA, batchA); err != nil {
		t.Fatalf("租户 A 批量落库失败: %v", err)
	}
	for i, rec := range batchA {
		if rec.TenantID != gprcTenantA {
			t.Fatalf("第 %d 行未填租户: %d", i, rec.TenantID)
		}
		if rec.ID == "" {
			t.Fatalf("第 %d 行未生成 ID", i)
		}
	}
	if err := repo.BatchCreate(ctxB, []*model.GeoPushRecord{{Platform: "indexnow", URL: "https://b.example/1"}}); err != nil {
		t.Fatalf("租户 B 落库失败: %v", err)
	}

	// 隔离 callback 按 ctx 遮挡：租户 A 只看得到自己那两行。
	var countA int64
	if err := db.WithContext(sysCtx).Model(&model.GeoPushRecord{}).Where("tenant_id = ?", gprcTenantA).Count(&countA).Error; err != nil {
		t.Fatalf("回查租户 A 记录数失败: %v", err)
	}
	if countA != 2 {
		t.Fatalf("租户 A 记录数 = %d，期望 2", countA)
	}
	var scopedA int64
	if err := db.WithContext(ctxA).Model(&model.GeoPushRecord{}).Count(&scopedA).Error; err != nil {
		t.Fatalf("租户 A 遮挡计数失败: %v", err)
	}
	if scopedA != 2 {
		t.Fatalf("租户 A 遮挡计数 = %d，期望 2", scopedA)
	}

	// fail-closed：on 模式下没有租户的 ctx 不允许写租户表。
	if err := repo.BatchCreate(context.Background(), []*model.GeoPushRecord{{Platform: "baidu", URL: "https://x.example/"}}); err == nil {
		t.Fatal("无租户 ctx 落库应报错，实际成功")
	}

	// 空批次不算错误（无推送结果时不写库）。
	if err := repo.BatchCreate(ctxA, nil); err != nil {
		t.Fatalf("空批次应直接返回，实际 err=%v", err)
	}
}
