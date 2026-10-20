package llm

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// 多租户下 trace 事件必须按事件自带租户分组落库。
//
// 两条错法都被这条用例挡住：
//   - 旧实现整批无租户上下文 ⇒ map create 绕过 Schema 校验，全部静默落租户 1；
//   - 只修 callback fail-closed 而不分组 ⇒ 一批里混租户，整批被拒、trace 全丢。
//
// 无归属事件（公开路由）必须 fail-closed，不得借默认租户蒙混落库。
func TestDBTraceSinkPersistsTenantScopedEvents(t *testing.T) {
	d := testutil.NewTestDB(t, &TraceEvent{})
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	sink := NewDBTraceSink(d)
	sink.Start()

	now := time.Now()
	sink.OnEvent(TraceEvent{TenantID: 2, TraceID: "tr-t2", SpanID: "sp-t2", Kind: TraceSpanKindLLMCall, Service: "llm", Operation: "chat_completion", Status: "ok", Timestamp: now})
	sink.OnEvent(TraceEvent{TenantID: 3, TraceID: "tr-t3", SpanID: "sp-t3", Kind: TraceSpanKindToolCall, Service: "tool", Operation: "send_email", Status: "ok", Timestamp: now})
	sink.OnEvent(TraceEvent{TraceID: "tr-unbound", SpanID: "sp-unbound", Kind: TraceSpanKindLog, Service: "http", Operation: "GET /health", Status: "ok", Timestamp: now})

	sink.Stop()

	var got []TraceEvent
	if err := d.WithContext(tenant.WithSystemScope(context.Background())).
		Where("span_id IN ?", []string{"sp-t2", "sp-t3", "sp-unbound"}).
		Order("span_id").Find(&got).Error; err != nil {
		t.Fatalf("回读 trace_events 失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应落 2 行（无归属行 fail-closed），实得 %d 行: %+v", len(got), got)
	}
	if got[0].SpanID != "sp-t2" || got[0].TenantID != 2 {
		t.Errorf("sp-t2 归属错误: span=%s tenant=%d", got[0].SpanID, got[0].TenantID)
	}
	if got[1].SpanID != "sp-t3" || got[1].TenantID != 3 {
		t.Errorf("sp-t3 归属错误: span=%s tenant=%d", got[1].SpanID, got[1].TenantID)
	}
	if inserted, dropped := sink.Stats(); inserted != 2 || dropped != 1 {
		t.Errorf("计数应为 inserted=2 dropped=1（无归属事件被跳过），实得 inserted=%d dropped=%d", inserted, dropped)
	}
}
