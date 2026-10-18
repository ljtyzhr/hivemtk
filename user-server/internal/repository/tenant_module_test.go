package repository

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// TestTenantModuleUsableCount UsableCount 的可用判据必须与
// model.TenantModuleUsable 同口径（active + 未过期），且空 key / 未知 key
// 不虚报 —— 它是 FR-5.6 实例级模块闸（service.AnyTenantEnabled）的唯一
// DB 判据，口径漂了闸就跟着漂。
//
// 写读一律挂 SystemScope：模块镜像属租户表，on 模式下无绑定的写/删会被
// 隔离 callback fail-closed（off 模式 SystemScope 是直通，SQL 形状不变）。
func TestTenantModuleUsableCount(t *testing.T) {
	database := testutil.NewTestDB(t, &model.TenantModule{})
	sysCtx := tenant.WithSystemScope(context.Background())
	truncate := func() {
		if err := database.Session(&gorm.Session{AllowGlobalUpdate: true}).
			WithContext(sysCtx).Delete(&model.TenantModule{}).Error; err != nil {
			t.Fatalf("清空 tenant_modules: %v", err)
		}
	}
	truncate()
	t.Cleanup(func() {
		if err := database.Session(&gorm.Session{AllowGlobalUpdate: true}).
			WithContext(tenant.WithSystemScope(context.Background())).Delete(&model.TenantModule{}).Error; err != nil {
			t.Logf("清理 tenant_modules 失败: %v", err)
		}
	})

	repo := NewTenantModuleRepositoryWithDB(database)
	ctx := sysCtx
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	// (tenant_id, module_key) 唯一：每个 key 只放一行，各形态错开
	rows := []*model.TenantModule{
		{ModuleKey: "uc_live_nolimit", Status: model.TenantModuleActive, ActivatedAt: now},
		{ModuleKey: "uc_live_future", Status: model.TenantModuleActive, ActivatedAt: now, ExpiresAt: &future},
		{ModuleKey: "uc_expired", Status: model.TenantModuleActive, ActivatedAt: past, ExpiresAt: &past},
		{ModuleKey: "uc_disabled", Status: model.TenantModuleDisabled, ActivatedAt: now},
	}
	if err := repo.UpsertMany(ctx, rows); err != nil {
		t.Fatalf("写入镜像: %v", err)
	}

	cases := []struct {
		key  string
		want int64
	}{
		{"uc_live_nolimit", 1},
		{"uc_live_future", 1},
		{"uc_expired", 0},    // status=active 但已过期 → 不可用
		{"uc_disabled", 0},   // status≠active
		{"uc_never_seen", 0}, // 未知 key
		{"", 0},              // 空 key 恒 0（调用方在上层放行）
	}
	for _, c := range cases {
		got, err := repo.UsableCount(ctx, c.key, now)
		if err != nil {
			t.Errorf("UsableCount(%q) 报错: %v", c.key, err)
			continue
		}
		if got != c.want {
			t.Errorf("UsableCount(%q) = %d, want %d", c.key, got, c.want)
		}
	}

	// 与 TenantModuleUsable 逐 key 对账（单口径教义：两处判据必须一起改）
	usableByModel := map[string]int64{}
	for _, r := range rows {
		if model.TenantModuleUsable(r, now) {
			usableByModel[r.ModuleKey]++
		}
	}
	for key, want := range usableByModel {
		got, err := repo.UsableCount(ctx, key, now)
		if err != nil || got != want {
			t.Errorf("UsableCount(%q)=%d,err=%v 与 TenantModuleUsable 计数 %d 不一致", key, got, err, want)
		}
	}
	if len(usableByModel) != 2 {
		t.Errorf("前置：TenantModuleUsable 应判 2 个 key 可用，实际 %d", len(usableByModel))
	}
}
