package tracing

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// TestFlushSpansRoutesByTenant 钉住异步落库的租户路由：
// 隔离 callback 用 ctx 绑定覆盖 tenant_id，所以一批混租户 span 必须拆组落库；
// 无归属的那组照旧 fail-closed，且不能把同批其他租户一起带走。
func TestFlushSpansRoutesByTenant(t *testing.T) {
	d := testutil.NewTestDB(t, &model.MessageTrace{})
	if d == nil {
		t.Fatal("NewTestDB 返回 nil（测试库不可达）")
	}
	orig := db.GetDB()
	db.SetTestDB(d)
	t.Cleanup(func() { db.SetTestDB(orig) })

	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	const suffix = "-tenant-routing"
	t2 := "tr-t2" + suffix
	t3 := "tr-t3" + suffix
	t0 := "tr-t0" + suffix
	flushSpans(d, []pendingSpan{
		{tenantID: 2, traceID: t2, node: NodeIngest, nodeOrder: 1, status: StatusOk},
		{tenantID: 0, traceID: t0, node: NodeIngest, nodeOrder: 1, status: StatusOk},
		{tenantID: 3, traceID: t3, node: NodeIngest, nodeOrder: 1, status: StatusOk},
	})

	var rows []model.MessageTrace
	sysCtx := tenant.WithSystemScope(context.Background())
	if err := d.WithContext(sysCtx).Where("trace_id IN ?", []string{t2, t3, t0}).
		Order("tenant_id asc").Find(&rows).Error; err != nil {
		t.Fatalf("查询落库行失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("落库行数=%d want 2（无租户那组应 fail-closed，且不得拖垮同批其他组）", len(rows))
	}
	if rows[0].TenantID != 2 || rows[0].TraceID != t2 {
		t.Errorf("第一组落错租户: %+v", rows[0])
	}
	if rows[1].TenantID != 3 || rows[1].TraceID != t3 {
		t.Errorf("第二组落错租户: %+v", rows[1])
	}
	var unbound int64
	if err := d.WithContext(sysCtx).Model(&model.MessageTrace{}).
		Where("trace_id = ?", t0).Count(&unbound).Error; err != nil {
		t.Fatalf("统计无归属行失败: %v", err)
	}
	if unbound != 0 {
		t.Errorf("无租户归属的 span 不应落库（on 模式必须 fail-closed），得到 %d 行", unbound)
	}
}

// TestPublishPropagatesTenant 钉住三条发布入口都把调用方 ctx 的租户写进 pendingSpan：
// 只修 flush 而不修捕获，行会被当成"无归属"整批拒绝。
func TestPublishPropagatesTenant(t *testing.T) {
	orig := spanCh
	spanCh = make(chan pendingSpan, 8)
	t.Cleanup(func() { spanCh = orig })

	ctx := tenant.WithTenantID(context.Background(), 7)
	RecordNode(ctx, NodeSpan{Node: NodeIngest})
	ReportToolCall(ctx, ToolTraceEvent{ToolName: "kb_search"})
	Start(ctx, NodeAIDispatch).End(nil, nil)

	for i := 0; i < 3; i++ {
		select {
		case p := <-spanCh:
			if p.tenantID != 7 {
				t.Errorf("第 %d 条 span 丢失租户归属: got %d want 7 (node=%s)", i+1, p.tenantID, p.node)
			}
			if m := toModelFromPending(p); m.TenantID != 7 {
				t.Errorf("第 %d 条 span 落模型时丢失 tenant_id: got %d want 7", i+1, m.TenantID)
			}
		default:
			t.Fatalf("第 %d 条 span 没有发布出来", i+1)
		}
	}
}
