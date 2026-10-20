package ragcache

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// 答案缓存是跨请求复用的派生数据：同 kb_id + 同向量在不同租户下必须互不可见，
// 且写入（Create map）必须由隔离 callback 自动填充 tenant_id。
func TestPGAnswerCacheStore_TenantIsolation(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &RAGAnswerCache{})
	store := NewPGAnswerCacheStore(db)

	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.001
	}
	kbUpdated := time.Now().Add(-time.Minute)

	ctxA := tenant.WithTenantID(context.Background(), 11)
	ctxB := tenant.WithTenantID(context.Background(), 22)
	ctxC := tenant.WithTenantID(context.Background(), 33)

	for ctx, answer := range map[context.Context]string{ctxA: "answer-A", ctxB: "answer-B"} {
		if err := store.Put(ctx, &Entry{
			KBID:          "kb-tenant",
			PromptVersion: "v1",
			QueryVector:   vec,
			Answer:        answer,
			KBUpdatedAt:   kbUpdated,
		}); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	gotA, err := store.GetExact(ctxA, "kb-tenant", "v1", vec)
	if err != nil {
		t.Fatalf("GetExact A: %v", err)
	}
	if gotA == nil || gotA.Answer != "answer-A" {
		t.Fatalf("A 读到错误答案: %+v", gotA)
	}
	gotB, err := store.GetExact(ctxB, "kb-tenant", "v1", vec)
	if err != nil {
		t.Fatalf("GetExact B: %v", err)
	}
	if gotB == nil || gotB.Answer != "answer-B" {
		t.Fatalf("B 读到错误答案（疑似串租户）: %+v", gotB)
	}

	gotC, err := store.GetExact(ctxC, "kb-tenant", "v1", vec)
	if err != nil {
		t.Fatalf("GetExact C: %v", err)
	}
	if gotC != nil {
		t.Fatalf("C 读到了其他租户的缓存: %+v", gotC)
	}

	// on 模式无租户绑定必须 fail-closed，不得退化为全表可见
	if _, err := store.GetExact(context.Background(), "kb-tenant", "v1", vec); err == nil {
		t.Fatal("无租户 ctx 的查询未被拒绝")
	}
}
