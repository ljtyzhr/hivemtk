package event

import (
	"context"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/tenant"
)

// A9（`未完成任务.md` §9）回归：事件总线的租户由发布口统一盖章。
//
// 立此文件的理由：T021 的"发布点从 ctx 填 TenantID"是**40+ 处各写一遍**的形状，
// 漏一处 ⇒ 订阅者拿到 0 ⇒ on 模式下要么 fail-closed 空转、要么落默认租户。
// 本组用例钉死三件事：
//  1. PublishCtx 会盖章（值类型、指针类型都盖）；
//  2. 已经填过的非 0 值**不覆盖**（"行自证"的发布点要能钉死行归属）；
//  3. 载荷没有 TenantID 字段、或 ctx 没租户 ⇒ 原样透传（不做隐式兜底）。

type tenantStampProbe struct {
	TenantID uint
	Other    string
}

type noTenantProbe struct {
	Other string
}

func TestStampTenantFromCtx_ValuePayload(t *testing.T) {
	ctx := tenant.WithTenantID(context.Background(), 5)
	got := StampTenantFromCtx(ctx, tenantStampProbe{Other: "x"})
	p, ok := got.(tenantStampProbe)
	if !ok {
		t.Fatalf("值类型载荷应当返回同类型副本，得到 %T", got)
	}
	if p.TenantID != 5 {
		t.Fatalf("TenantID 应当被盖成 5，实际 %d", p.TenantID)
	}
	if p.Other != "x" {
		t.Fatalf("其余字段不得改动，实际 %+v", p)
	}
}

func TestStampTenantFromCtx_PointerPayload(t *testing.T) {
	ctx := tenant.WithTenantID(context.Background(), 7)
	p := &tenantStampProbe{}
	got := StampTenantFromCtx(ctx, p)
	if got.(*tenantStampProbe).TenantID != 7 {
		t.Fatalf("指针载荷应当被就地盖章，实际 %+v", p)
	}
}

func TestStampTenantFromCtx_DoesNotOverride(t *testing.T) {
	ctx := tenant.WithTenantID(context.Background(), 5)
	got := StampTenantFromCtx(ctx, tenantStampProbe{TenantID: 9})
	if p := got.(tenantStampProbe); p.TenantID != 9 {
		t.Fatalf("已填的 TenantID 不得被覆盖（行自证优先），实际 %d", p.TenantID)
	}
}

func TestStampTenantFromCtx_NoTenantFieldOrNoCtxTenant(t *testing.T) {
	// 载荷无 TenantID 字段：原样返回（这类事件与租户无关）
	if got := StampTenantFromCtx(tenant.WithTenantID(context.Background(), 5), noTenantProbe{Other: "y"}); got.(noTenantProbe).Other != "y" {
		t.Fatalf("无 TenantID 字段的载荷必须原样透传")
	}
	// ctx 无租户：不兜底（off 模式 = 逐字节等价；on 模式 = 交给响声，不猜）
	got := StampTenantFromCtx(context.Background(), tenantStampProbe{})
	if p := got.(tenantStampProbe); p.TenantID != 0 {
		t.Fatalf("ctx 无租户时不得凭空盖租户，实际 %d", p.TenantID)
	}
	// nil 载荷 / nil ctx 不 panic
	if got := StampTenantFromCtx(context.Background(), nil); got != nil {
		t.Fatalf("nil 载荷应原样返回")
	}
	if got := StampTenantFromCtx(nil, tenantStampProbe{}); got.(tenantStampProbe).TenantID != 0 { //nolint:staticcheck // 显式验证 nil ctx 不 panic
		t.Fatalf("nil ctx 应原样返回")
	}
}

// TestPublishCtx_ReachesSubscriberWithTenant 端到端：订阅者拿到的载荷带租户。
func TestPublishCtx_ReachesSubscriberWithTenant(t *testing.T) {
	prev := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOn)
	defer tenant.SetMode(prev)

	bus := New(1, 16)
	defer bus.Stop()

	var got uint
	var seen bool
	var mu sync.Mutex
	bus.Subscribe(TopicKnowledgeDocumentChanged, func(evt Event) error {
		if p, ok := evt.Payload.(KnowledgeDocumentChangePayload); ok {
			mu.Lock()
			got, seen = p.TenantID, true
			mu.Unlock()
		}
		return nil
	})

	bus.Publish(Event{
		Topic:   TopicKnowledgeDocumentChanged,
		Payload: StampTenantFromCtx(tenant.WithTenantID(context.Background(), 11), KnowledgeDocumentChangePayload{}),
	})

	waitForCondition(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return seen
	}, 500*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if !seen {
		t.Fatalf("订阅者未在超时内收到事件")
	}
	if got != 11 {
		t.Fatalf("订阅者应当看到 TenantID=11，实际 %d", got)
	}
}
