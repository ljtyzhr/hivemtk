package agent_runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/event"
	"hivemtk-user/internal/pkg/tenant"
)

var _ = strings.Contains

// TestPublishKnowledgeDocumentChange_AutoTraceID 验证空 traceID 自动生成
func TestPublishKnowledgeDocumentChange_AutoTraceID(t *testing.T) {
	traceID := PublishKnowledgeDocumentChange(context.Background(), "1", 100, "create", "test content", 1, "")
	if traceID == "" {
		t.Error("traceID should be auto-generated when empty")
	}
	if !strings.Contains(traceID, "rag_") {
		t.Errorf("traceID = %s, should contain 'rag_'", traceID)
	}
}

// TestPublishKnowledgeDocumentChange_CustomTraceID 验证自定义 traceID 保留
func TestPublishKnowledgeDocumentChange_CustomTraceID(t *testing.T) {
	custom := "custom_trace_001"
	traceID := PublishKnowledgeDocumentChange(context.Background(), "1", 100, "create", "test", 1, custom)
	if traceID != custom {
		t.Errorf("traceID = %s, want %s", traceID, custom)
	}
}

// TestPublishKnowledgeDocumentChange_CarriesTenant 验证租户归属随载荷随行传递。
// 订阅者跑在事件线程上拿不到发布点的 ctx，载荷不带租户时，on 模式下索引器会以
// 无绑定上下文读写知识库（fail-closed），整条增量索引链静默失效。
func TestPublishKnowledgeDocumentChange_CarriesTenant(t *testing.T) {
	bus := event.New(2, 100)
	defer bus.Stop()
	prevGlobal := event.GetGlobalBus()
	event.SetGlobalBus(bus)
	defer event.SetGlobalBus(prevGlobal)

	captured := make(chan event.Event, 1)
	bus.Subscribe(event.TopicKnowledgeDocumentChanged, func(evt event.Event) error {
		captured <- evt
		return nil
	})

	PublishKnowledgeDocumentCreate(tenant.WithTenantID(context.Background(), 7), "1", 100, "test content", 1)

	select {
	case evt := <-captured:
		payload, ok := evt.Payload.(event.KnowledgeDocumentChangePayload)
		if !ok {
			t.Fatalf("payload 类型 = %T，期望 KnowledgeDocumentChangePayload", evt.Payload)
		}
		if payload.TenantID != 7 {
			t.Errorf("payload.TenantID = %d, want 7", payload.TenantID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("事件总线上没有收到知识库变更事件")
	}
}

// TestPublishKnowledgeDocumentCreate 验证 create 快捷方法
func TestPublishKnowledgeDocumentCreate(t *testing.T) {
	traceID := PublishKnowledgeDocumentCreate(context.Background(), "1", 100, "test content", 1)
	if traceID == "" {
		t.Error("traceID should not be empty")
	}
}

// TestPublishKnowledgeDocumentUpdate 验证 update 快捷方法
func TestPublishKnowledgeDocumentUpdate(t *testing.T) {
	traceID := PublishKnowledgeDocumentUpdate(context.Background(), "1", 100, "updated content", 1)
	if traceID == "" {
		t.Error("traceID should not be empty")
	}
}

// TestPublishKnowledgeDocumentDelete 验证 delete 快捷方法
func TestPublishKnowledgeDocumentDelete(t *testing.T) {
	traceID := PublishKnowledgeDocumentDelete(context.Background(), "1", 100, 1)
	if traceID == "" {
		t.Error("traceID should not be empty")
	}
}

// TestKnowledgeDocumentChangePayload 验证 payload 字段
func TestKnowledgeDocumentChangePayload(t *testing.T) {
	payload := event.KnowledgeDocumentChangePayload{
		WorkspaceID: "1",
		DocumentID:  100,
		ChangeType:  "create",
		ContentHash: "abc123",
		OperatorID:  1,
		TraceID:     "trace_001",
	}

	if payload.WorkspaceID != "1" {
		t.Errorf("WorkspaceID = %v, want \"1\"", payload.WorkspaceID)
	}
	if payload.DocumentID != 100 {
		t.Errorf("DocumentID = %d, want 100", payload.DocumentID)
	}
	if payload.ChangeType != "create" {
		t.Errorf("ChangeType = %s, want create", payload.ChangeType)
	}
}
