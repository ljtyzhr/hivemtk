package repository

// 契约：短信配置（以及任何"读出旧行 → 把新值盖回去"的写路径）**不得改写 tenant_id**。
//
// 坑：仓储的写法是"先 First 读出 existing，再把新 model 的 ID 认成 existing.ID，然后 Save"。
// GORM 的 Save（Selects=* 形态）写**全部字段**、含零值 —— 而调用方传进来的是**新建的零值
// model**（tenant_id=0），于是这一行的租户被改写成 0。
// 多租户模式下 tenant 回调是 fail-closed 的，当场拒掉：
//
//	[tenant] 更新不可改写 tenant_id（上下文租户 1，SET 值 0）
//
// 独立部署模式（TENANT_MODE=off）没有这层校验，于是同一个接口在 SaaS 下 500、
// 私域下 200 —— 正是多租户改造要暴露的那类差异（而且失败方向是"能用的那边掩盖了坏的这边"）。
//
// 修法：Save 之前把库里的 tenant_id 带回来。
//
// 本文件锁的是"多租户模式下这一行仍在原租户名下"：
//  1. 读旧行 → 改字段 → Save，tenant_id 必须不变；
//  2. 新建一行（库里没有）→ Create，tenant_id 必须落在上下文租户上（不是 0、不是别的租户）。

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
)

// withMultiTenantMode 打开多租户模式并在收尾时还原。
func withMultiTenantMode(t *testing.T) {
	t.Helper()
	orig := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(orig) })
}

// readSmsConfigTenantID 独立零值 struct 读回（复用已填充 struct 会把旧字段并进 WHERE）。
func readSmsConfigTenantID(t *testing.T, db *gorm.DB, id, tid uint) uint {
	t.Helper()
	var got model.SmsConfig
	ctx := tenant.WithTenantID(context.Background(), tid)
	if err := db.WithContext(ctx).Where("id = ?", id).First(&got).Error; err != nil {
		t.Fatalf("读回短信配置 %d 失败：%v", id, err)
	}
	return got.TenantID
}

// TestSmsRepositorySaveConfigKeepsTenantID 多租户下保存配置不得改写租户归属。
func TestSmsRepositorySaveConfigKeepsTenantID(t *testing.T) {
	withMultiTenantMode(t)
	database := setupSmsTestDB(t)
	repo := NewSmsRepository()

	const tid = uint(7)
	ctx := tenant.WithTenantID(context.Background(), tid)

	// 归租户这一步本身要带租户上下文（update 受 scope 保护）。
	// 用「先按 system scope 建行、再用目标租户的 ctx 改租户」两段式不成立
	//（改 tenant_id 属于改租户归属，回调会拒），改为直接在目标租户 ctx 下建行。
	if err := database.WithContext(ctx).Create(&model.SmsConfig{
		DefaultProvider: "aliyun", RateLimit: 100, DailyLimit: 1000, RetryTimes: 1,
	}).Error; err != nil {
		t.Fatalf("种子配置落库失败：%v", err)
	}
	var seedRow model.SmsConfig
	if err := database.WithContext(ctx).First(&seedRow).Error; err != nil {
		t.Fatalf("读回种子配置失败：%v", err)
	}

	// 模拟 controller→service 的真实入参：**新建的零值 model**（tenant_id=0）。
	if err := repo.SaveConfig(ctx, &model.SmsConfig{
		DefaultProvider: "huawei", RateLimit: 200, DailyLimit: 2000, RetryTimes: 3,
	}); err != nil {
		t.Fatalf("多租户模式下 SaveConfig 失败：%v", err)
	}

	if got := readSmsConfigTenantID(t, database, seedRow.ID, tid); got != tid {
		t.Errorf("tenant_id 应仍为 %d，实际 %d —— Save 把零值 tenant_id 一起写进去了，"+
			"这行要么被 tenant 回调拒掉、要么被静默改到别的租户", tid, got)
	}
}

// TestSmsRepositorySaveConfigCreatesInCurrentTenant 库里没有该配置时，新建行必须落在上下文租户。
func TestSmsRepositorySaveConfigCreatesInCurrentTenant(t *testing.T) {
	withMultiTenantMode(t)
	database := setupSmsTestDB(t)
	repo := NewSmsRepository()

	const tid = uint(9)
	ctx := tenant.WithTenantID(context.Background(), tid)

	// 清掉种子配置，确保走 Create 分支。
	if err := database.WithContext(tenant.WithSystemScope(context.Background())).
		Where("1 = 1").Delete(&model.SmsConfig{}).Error; err != nil {
		t.Fatalf("清空短信配置失败：%v", err)
	}

	if err := repo.SaveConfig(ctx, &model.SmsConfig{
		DefaultProvider: "aliyun", RateLimit: 10, DailyLimit: 100, RetryTimes: 1,
	}); err != nil {
		t.Fatalf("多租户模式下 SaveConfig（新建）失败：%v", err)
	}

	var got model.SmsConfig
	if err := database.WithContext(ctx).First(&got).Error; err != nil {
		t.Fatalf("读回短信配置失败：%v", err)
	}
	if got.TenantID != tid {
		t.Errorf("新建配置的 tenant_id 应为上下文租户 %d，实际 %d", tid, got.TenantID)
	}
}
