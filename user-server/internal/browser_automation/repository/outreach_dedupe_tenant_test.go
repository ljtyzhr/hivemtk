package repository

// 契约锁（实体审查 #7，TENANT_MODE=on）：触达去重的键位属于租户，不属于平台。
//
// 这张表的形状是「模型 tag 建四元唯一键 (platform, target_url, action, copy_hash)，
// 再由租户唯一索引规范化（tenancyidx.Normalize，与迁移 v3.59.0 同一实现）把它升格成
// 带 tenant_id 的复合键」。写侧用的是 `OnConflict{DoNothing}`，于是这条规范化一旦被漏掉，
// 失效形态**不是报错而是静默**：
//   - 租户 B 触达 A 已触达过的同一目标 ⇒ 冲突被 DoNothing 吞掉，B 一行都没落，
//     而 B 的 FindOutreachHit 因为行级隔离读不到 A 的行 ⇒ B 从此每次 cron 都重发同一条评论
//     （防扰民层对 B 永久失效），且库里没有任何痕迹说明「它本该被去重」。
//   - 反过来若读侧漏了租户过滤，B 会读到 A 的行 ⇒ B 以为「已触达过」而跳过，
//     界面上是 skipped，谁也不知道是被别人的记录挡掉的。
//
// 所以两条都要钉，且判据取物理行数：只有「B 确实插进了自己的那一行」才能同时证明
// 键位放开了、读侧挡住了。种子仍走被测的两个方法（不是直插 db），因为冲突吞掉这件事
// 只有经过 DoNothing 才看得见。

import (
	"context"
	"testing"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	dedupeTenantA = uint(16)
	dedupeTenantB = uint(37)
	// 两租户共用同一个四元键：这正是「同一平台同一目标同一动作同一文案」的真实形态——
	// 两家客户在同一篇笔记下发同一条话术，去重必须各自成立。
	dedupePlatform  = "xiaohongshu"
	dedupeTargetURL = "https://x.test/p/9001"
	dedupeAction    = "post_comment"
	dedupeCopyHash  = "shared-copy-hash"
	dedupeTaskA     = uint(792701)
	dedupeTaskB     = uint(792702)
)

func dedupeDB(t *testing.T) (*gorm.DB, context.Context, context.Context, BrowserOutreachDedupeRepository) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BrowserOutreachDedupe{})
	if db == nil {
		t.Fatal("测试库不可达：去重键位是否含租户无法判定（不 Skip，跳过等于没锁）")
	}
	return db,
		tenant.WithTenantID(context.Background(), dedupeTenantA),
		tenant.WithTenantID(context.Background(), dedupeTenantB),
		NewBrowserOutreachDedupeRepositoryWithDB(db)
}

func dedupeRecord(platform string, taskID uint) *model.BrowserOutreachDedupe {
	return &model.BrowserOutreachDedupe{
		Platform: platform, TargetURL: dedupeTargetURL, Action: dedupeAction,
		CopyHash: dedupeCopyHash, TaskID: taskID, SessionID: taskID + 1,
	}
}

func dedupePhysicalRows(t *testing.T, db *gorm.DB, platform string) int64 {
	t.Helper()
	var n int64
	// system scope + Unscoped：这里数的是「库里到底有几行」，与被隔离的读侧无关。
	err := db.WithContext(tenant.WithSystemScope(context.Background())).Unscoped().
		Model(&model.BrowserOutreachDedupe{}).Where("platform = ?", platform).Count(&n).Error
	if err != nil {
		t.Fatalf("物理行数读取失败: %v", err)
	}
	return n
}

// TestDedupeKeyIsPerTenant 同一四元键在两个租户各成立一条：B 读不到 A 的、也插得进自己的。
func TestDedupeKeyIsPerTenant(t *testing.T) {
	db, ctxA, ctxB, repo := dedupeDB(t)

	if err := repo.RecordOutreachSend(ctxA, dedupeRecord(dedupePlatform, dedupeTaskA)); err != nil {
		t.Fatalf("租户 %d 记录触达失败: %v", dedupeTenantA, err)
	}

	// ① B 的读侧必须 miss：A 的行不是 B 的去重依据。
	hit, err := repo.FindOutreachHit(ctxB, dedupePlatform, dedupeTargetURL, dedupeAction, dedupeCopyHash)
	if err != nil {
		t.Fatalf("租户 %d 查询失败: %v", dedupeTenantB, err)
	}
	if hit != nil {
		t.Errorf("租户 %d 命中了租户 %d 的触达行（task=%d）：B 会被判「已触达过」而 skip，"+
			"界面读起来正常、实际是被别人的记录挡掉的", dedupeTenantB, hit.TenantID, hit.TaskID)
	}

	// ② B 的写侧必须落得进去：键位若不含 tenant_id，这一句被 DoNothing 静默吞掉。
	if err := repo.RecordOutreachSend(ctxB, dedupeRecord(dedupePlatform, dedupeTaskB)); err != nil {
		t.Errorf("租户 %d 记录同一四元键失败: %v（唯一键未含 tenant_id 的形态）", dedupeTenantB, err)
	}
	if n := dedupePhysicalRows(t, db, dedupePlatform); n != 2 {
		t.Fatalf("物理行数 got=%d want=2：DoNothing 把 B 的行吞了，B 从此每次调度都重发同一条内容，"+
			"而库里看不出它本该被去重", n)
	}

	// ③ 落库后 B 才命中，且命中的是自己那条（归因 task_id 不许串）。
	hitB, err := repo.FindOutreachHit(ctxB, dedupePlatform, dedupeTargetURL, dedupeAction, dedupeCopyHash)
	if err != nil {
		t.Fatalf("租户 %d 二次查询失败: %v", dedupeTenantB, err)
	}
	if hitB == nil {
		t.Fatalf("租户 %d 记录后仍读不到自己的触达行：去重对 B 完全失效", dedupeTenantB)
	}
	if hitB.TaskID != dedupeTaskB || hitB.TenantID != dedupeTenantB {
		t.Errorf("租户 %d 命中行 got=(tenant=%d task=%d) want=(%d/%d)",
			dedupeTenantB, hitB.TenantID, hitB.TaskID, dedupeTenantB, dedupeTaskB)
	}
	hitA, err := repo.FindOutreachHit(ctxA, dedupePlatform, dedupeTargetURL, dedupeAction, dedupeCopyHash)
	if err != nil {
		t.Fatalf("租户 %d 二次查询失败: %v", dedupeTenantA, err)
	}
	if hitA == nil || hitA.TaskID != dedupeTaskA {
		t.Errorf("租户 %d 应命中自己那条（task=%d），got %+v", dedupeTenantA, dedupeTaskA, hitA)
	}
}

// TestDedupeSameTenantRerunStaysIdempotent 同租户重复记录仍只一行：
// 这一腿防的是「为了补租户维度把唯一键整个拆掉」——那样 B 插得进了，A 也重复插得进，
// 去重层反过来失效。
func TestDedupeSameTenantRerunStaysIdempotent(t *testing.T) {
	db, ctxA, _, repo := dedupeDB(t)
	for i := 0; i < 3; i++ {
		if err := repo.RecordOutreachSend(ctxA, dedupeRecord(dedupePlatform, dedupeTaskA)); err != nil {
			t.Fatalf("第 %d 次记录失败: %v", i+1, err)
		}
	}
	if n := dedupePhysicalRows(t, db, dedupePlatform); n != 1 {
		t.Errorf("同租户重复记录后物理行数 got=%d want=1：唯一键没能挡住同租户的重复触达", n)
	}
}

// TestDedupeRejectsUnboundContext 两个方法在裸 ctx 下都必须 fail-closed。
// 执行器那条链路（HTTP/cron/重试/MCP/workflow）都绑了租户，但「都绑了」是这一版的读数，
// 不是约束：一旦有人新加一条不绑租户的派发口，读侧会变成跨租户命中（skip 别人的记录），
// 写侧会变成把行记到不存在的租户名下。
func TestDedupeRejectsUnboundContext(t *testing.T) {
	_, _, _, repo := dedupeDB(t)
	bare := context.Background()

	if _, err := repo.FindOutreachHit(bare, dedupePlatform, dedupeTargetURL, dedupeAction, dedupeCopyHash); err == nil {
		t.Error("裸 ctx 的 FindOutreachHit 未被拒绝：这条读路径会扫全部租户的去重行")
	}
	if err := repo.RecordOutreachSend(bare, dedupeRecord(dedupePlatform, dedupeTaskA)); err == nil {
		t.Error("裸 ctx 的 RecordOutreachSend 未被拒绝：这一行落不进任何租户")
	}
}
