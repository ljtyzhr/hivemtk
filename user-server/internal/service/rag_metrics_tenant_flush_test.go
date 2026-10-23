// rag_metrics_tenant_flush_test.go 召回日志的**跨租户刷盘**口径。
//
// 与 reach_compliance 那组同一条缺口：`rag_query_logs` 是租户表，日志在
// RequestQuery 那一刻入队、在几秒后的后台 goroutine 里刷盘。原先 buildQueryLog
// 不抄租户、flush 用一个进程级 ctx 整批写 ⇒ on 模式整批被隔离 callback 拒掉
// （表现为"召回日志从来没落过库"），off 模式全部落默认租户。
package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

func ragQueryReq(q string) *RecordQueryRequest {
	return &RecordQueryRequest{
		Query:           q,
		RetrievedDocIDs: []string{"d1", "d2"},
		RelevantDocIDs:  []string{"d1"},
		Latency:         12,
		TopK:            5,
		Source:          "hybrid",
	}
}

// TestRagMetrics_FlushSplitsByTenant 两个租户各记一条 ⇒ 刷盘后各归各的租户。
func TestRagMetrics_FlushSplitsByTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := testutil.NewTestDB(t, &model.RagQueryLog{})
	svc := NewRagMetricsService(db)

	svc.RecordQuery(complianceTenantCtx(21), ragQueryReq("q-21"))
	svc.RecordQuery(complianceTenantCtx(22), ragQueryReq("q-22"))

	if err := svc.flush(context.Background()); err != nil {
		t.Fatalf("flush 失败: %v", err)
	}

	var mine []model.RagQueryLog
	if err := db.WithContext(complianceTenantCtx(21)).Find(&mine).Error; err != nil {
		t.Fatalf("读回租户 21 失败: %v", err)
	}
	if len(mine) != 1 || mine[0].Query != "q-21" {
		t.Fatalf("租户 21 名下应恰好 1 条 q-21，实际 %+v", mine)
	}
	if mine[0].TenantID != 21 {
		t.Errorf("租户 21 的行 tenant_id = %d，期望 21", mine[0].TenantID)
	}

	var theirs []model.RagQueryLog
	if err := db.WithContext(complianceTenantCtx(22)).Find(&theirs).Error; err != nil {
		t.Fatalf("读回租户 22 失败: %v", err)
	}
	if len(theirs) != 1 || theirs[0].Query != "q-22" {
		t.Fatalf("租户 22 名下应恰好 1 条 q-22，实际 %+v", theirs)
	}
	if theirs[0].TenantID != 22 {
		t.Errorf("租户 22 的行 tenant_id = %d，期望 22（分组刷盘把租户绑错了）", theirs[0].TenantID)
	}
}

// TestRagMetrics_DropsLogWithoutTenant on 模式下 RecordQuery 的 ctx 没有租户绑定
// ⇒ 这条日志在刷盘时被丢弃并出声，不落默认租户。
func TestRagMetrics_DropsLogWithoutTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := testutil.NewTestDB(t, &model.RagQueryLog{})
	svc := NewRagMetricsService(db)

	svc.RecordQuery(context.Background(), ragQueryReq("unbound"))
	svc.RecordQuery(complianceTenantCtx(21), ragQueryReq("q-21"))

	if err := svc.flush(context.Background()); err != nil {
		t.Fatalf("flush 失败: %v", err)
	}

	var rows []model.RagQueryLog
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Find(&rows).Error; err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("总行数 = %d，期望 1（无归属那条必须被丢弃，而不是落默认租户）", len(rows))
	}
	if rows[0].TenantID != 21 || rows[0].Query != "q-21" {
		t.Fatalf("保留下来的行 = %+v，期望租户 21 的 q-21", rows[0])
	}
}
