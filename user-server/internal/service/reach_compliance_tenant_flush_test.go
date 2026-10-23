// reach_compliance_tenant_flush_test.go 合规审计日志的**跨租户刷盘**口径。
//
// 钉的是一条 2026-10-24 才补上的缺口：`reach_compliance_log` 是租户表，而刷盘发生在
// 入队几秒后的后台 goroutine —— 那里只有进程级 ctx。原先 record 只收
// `(channel, recipientID)`，Flush 用 `context.Background()` 整批写：
//   - on 模式：无租户绑定 ⇒ 隔离 callback fail-closed ⇒ 整批一行都落不了库，
//     而报错只出现在刷盘日志里，看起来像"合规日志功能没开"；
//   - off 模式：整批落列默认值 1 —— 若进程同时服务多个租户，就是串库。
//
// 修法两条：入队时把租户抄进行里（`tenant.WriteTenantID`），刷盘时按行里的租户
// 分组、每组 `tenant.OnlyTenant` 显式绑定。下面的用例逐条钉住这两条。
package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

func complianceTenantCtx(id uint) context.Context {
	return tenant.WithTenantID(context.Background(), id)
}

// TestComplianceAuditLogger_FlushSplitsByTenant 一批缓冲里混着两个租户 ⇒ 各落各的租户。
func TestComplianceAuditLogger_FlushSplitsByTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := testutil.NewTestDB(t, &model.ReachComplianceLog{})
	l := &ComplianceAuditLogger{repo: repository.NewReachComplianceLogRepository(db)}

	l.record(complianceTenantCtx(21), "sms", "u-21")
	l.record(complianceTenantCtx(22), "wecom", "u-22")
	l.record(complianceTenantCtx(22), "email", "u-22b")

	if got := l.BufferedCount(); got != 3 {
		t.Fatalf("缓冲条数 = %d，期望 3", got)
	}
	if err := l.Flush(); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if got := l.BufferedCount(); got != 0 {
		t.Fatalf("刷盘成功后缓冲应清零，实际 %d（回队逻辑把成功的行也塞回去了）", got)
	}

	// 逐租户读回：读口本身也走隔离 callback，读到别租户的行会当场露出来。
	var mine []ReachComplianceLog
	if err := db.WithContext(complianceTenantCtx(21)).Order("id ASC").Find(&mine).Error; err != nil {
		t.Fatalf("读回租户 21 失败: %v", err)
	}
	if len(mine) != 1 || mine[0].RecipientID != "u-21" {
		t.Fatalf("租户 21 名下应恰好 1 条 u-21，实际 %+v", mine)
	}
	if mine[0].TenantID != 21 {
		t.Errorf("租户 21 那一行 tenant_id = %d，期望 21", mine[0].TenantID)
	}

	var theirs []ReachComplianceLog
	if err := db.WithContext(complianceTenantCtx(22)).Order("id ASC").Find(&theirs).Error; err != nil {
		t.Fatalf("读回租户 22 失败: %v", err)
	}
	if len(theirs) != 2 {
		t.Fatalf("租户 22 名下应 2 条，实际 %d：%+v", len(theirs), theirs)
	}
	for _, r := range theirs {
		if r.TenantID != 22 {
			t.Errorf("租户 22 的行 tenant_id = %d，期望 22（分组刷盘把租户绑错了）", r.TenantID)
		}
	}

	// 跨租户 union 总数也必须是 3：分组刷盘若整批用了某一组的 ctx，
	// 另外一组会被 callback 拒掉，总数就会少。
	var total int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.ReachComplianceLog{}).Count(&total).Error; err != nil {
		t.Fatalf("system scope 统计失败: %v", err)
	}
	if total != 3 {
		t.Fatalf("跨租户总行数 = %d，期望 3（少于 3 说明有一组被整批拒掉了）", total)
	}
}

// TestComplianceAuditLogger_DropsRowWithoutTenant on 模式下入队时没有租户绑定
// ⇒ 该行在刷盘时被丢弃并出声，绝不落默认租户。
func TestComplianceAuditLogger_DropsRowWithoutTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := testutil.NewTestDB(t, &model.ReachComplianceLog{})
	l := &ComplianceAuditLogger{repo: repository.NewReachComplianceLogRepository(db)}

	l.record(context.Background(), "sms", "unbound") // on 模式无绑定 ⇒ TenantID 0
	l.record(complianceTenantCtx(21), "sms", "u-21")

	if err := l.Flush(); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	var total int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.ReachComplianceLog{}).Count(&total).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("总行数 = %d，期望 1（无归属那行必须被丢弃，而不是落默认租户）", total)
	}
	var rows []ReachComplianceLog
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Find(&rows).Error; err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if rows[0].TenantID != 21 || rows[0].RecipientID != "u-21" {
		t.Fatalf("保留下来的行 = %+v，期望租户 21 的 u-21", rows[0])
	}
}
