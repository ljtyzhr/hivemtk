package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// 跨租户越权回归（specs/001-saas-multi-tenant US3/T028）：
// 双租户 fixture × 核心域写读交叉断言 0 泄漏（服务层最小集，
// 全 API 域扫描由 callback 七态 + 路由闸 + 域测试分层共同承担）。
func TestTenantIsolationCrossTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	database := testutil.NewTestDB(t, &model.Customer{}, &model.ChatChannel{})

	ctxA := tenant.WithTenantID(context.Background(), 11)
	ctxB := tenant.WithTenantID(context.Background(), 22)

	// A 写两行、B 写一行
	for _, name := range []string{"a1", "a2"} {
		if err := database.WithContext(ctxA).Create(&model.Customer{Name: name}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.WithContext(ctxB).Create(&model.Customer{Name: "b1"}).Error; err != nil {
		t.Fatal(err)
	}

	// ① 列表：A/B 各只见自己的
	var nA, nB int64
	if err := database.WithContext(ctxA).Model(&model.Customer{}).Count(&nA).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctxB).Model(&model.Customer{}).Count(&nB).Error; err != nil {
		t.Fatal(err)
	}
	if nA != 2 || nB != 1 {
		t.Fatalf("列表隔离破：A=%d(期望2) B=%d(期望1)", nA, nB)
	}

	// ② 伪造跨租户资源 ID 直查：表现为"不存在"
	var got model.Customer
	// A 用 B 的行 id（先查出 b1 的真实 id 再用 A 的 ctx 查）
	var b1 model.Customer
	if err := database.WithContext(ctxB).Where("name = ?", "b1").First(&b1).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctxA).First(&got, "id = ?", b1.ID).Error; err == nil {
		t.Fatal("A 直查 B 的资源 ID 必须不可见（表现为不存在）")
	}

	// ③ 跨租户更新：受影响行数必须为 0
	res := database.WithContext(ctxA).Model(&model.Customer{}).
		Where("id = ?", b1.ID).Update("name", "hacked")
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	if res.RowsAffected != 0 {
		t.Fatalf("跨租户更新被放行 %d 行", res.RowsAffected)
	}

	// ④ 跨租户删除：受影响行数必须为 0
	res = database.WithContext(ctxA).Where("id = ?", b1.ID).Delete(&model.Customer{})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	if res.RowsAffected != 0 {
		t.Fatalf("跨租户删除被放行 %d 行", res.RowsAffected)
	}

	// ⑤ B 的数据完好
	var still int64
	if err := database.WithContext(ctxB).Model(&model.Customer{}).Count(&still).Error; err != nil {
		t.Fatal(err)
	}
	if still != 1 {
		t.Fatalf("B 数据被破坏: %d", still)
	}

	// ⑥ 渠道域：channel 行自证租户后列表分域
	if err := database.WithContext(ctxA).Create(&model.ChatChannel{ChannelID: "ch-a", TenantID: 11}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctxB).Create(&model.ChatChannel{ChannelID: "ch-b", TenantID: 22}).Error; err != nil {
		t.Fatal(err)
	}
	var chA, chB int64
	if err := database.WithContext(ctxA).Model(&model.ChatChannel{}).Count(&chA).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctxB).Model(&model.ChatChannel{}).Count(&chB).Error; err != nil {
		t.Fatal(err)
	}
	if chA != 1 || chB != 1 {
		t.Fatalf("渠道域隔离破：A=%d B=%d", chA, chB)
	}
}
