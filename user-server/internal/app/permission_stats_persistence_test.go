// permission_stats_persistence_test.go D-2 的落库契约（不依赖 PG）：
// flush 后新实例 load 能读回、周期 flush 真的会跑、读失败时宁可不写也不覆盖历史。
package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
)

// fakeToolRiskStatStore 是 platform_config_kv 的内存替身。
type fakeToolRiskStatStore struct {
	mu        sync.Mutex
	values    map[string]string
	available bool
	getErr    error
	upserts   int
}

func newFakeToolRiskStatStore() *fakeToolRiskStatStore {
	return &fakeToolRiskStatStore{values: map[string]string{}, available: true}
}

func (f *fakeToolRiskStatStore) Available() bool { return f != nil && f.available }

func (f *fakeToolRiskStatStore) Get(ctx context.Context, key string) (string, error) {
	if !f.Available() {
		return "", errors.New("store unavailable")
	}
	if f.getErr != nil {
		return "", f.getErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[key], nil
}

func (f *fakeToolRiskStatStore) Upsert(ctx context.Context, key, value string) (string, error) {
	if !f.Available() {
		return "", errors.New("store unavailable")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] = value
	f.upserts++
	return value, nil
}

func (f *fakeToolRiskStatStore) raw(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[key]
}

func TestToolRiskStats_FlushThenLoadRoundTrip(t *testing.T) {
	store := newFakeToolRiskStatStore()
	ctx := context.Background()
	now := time.Now()

	obs1 := tooluse.NewMemoryRiskObserver(10)
	obs1.Observe(ctx, tooluse.RiskDecision{
		ToolName: "reach.sms.send", Level: tooluse.RiskHighWrite, Declared: true,
		WouldDeny: true, Reason: tooluse.RiskReasonWhitelistAbsent, At: now,
	})
	obs1.Observe(ctx, tooluse.RiskDecision{
		ToolName: "reach.sms.send", Level: tooluse.RiskHighWrite, Declared: true,
		WouldDeny: true, Reason: tooluse.RiskReasonWhitelistAbsent, At: now.Add(time.Second),
	})
	obs1.Observe(ctx, tooluse.RiskDecision{
		ToolName: "reach.sms.send", Level: tooluse.RiskHighWrite, Declared: true,
		WouldDeny: true, AgentWildcard: true, Reason: tooluse.RiskReasonNotGranted, At: now.Add(2 * time.Second),
	})
	obs1.Observe(ctx, tooluse.RiskDecision{
		ToolName: "reach.sms.send", Level: tooluse.RiskHighWrite, Declared: true,
		WouldDeny: false, Reason: tooluse.RiskReasonGranted, At: now.Add(3 * time.Second),
	})

	if err := flushToolRiskStats(ctx, obs1, store); err != nil {
		t.Fatalf("flush 失败: %v", err)
	}
	if store.raw(ToolRiskStatsKVKey) == "" {
		t.Fatal("flush 后 KV 里没有数据")
	}

	// 模拟重启：新 observer 从同一份 KV 读回。
	obs2 := tooluse.NewMemoryRiskObserver(10)
	n, err := loadToolRiskStats(ctx, obs2, store)
	if err != nil {
		t.Fatalf("load 失败: %v", err)
	}
	if n == 0 {
		t.Fatal("load 报告 0 行，但库里应有 3 条 (tool,decision,reason,wildcard) 聚合")
	}
	got := obs2.Counts()["reach.sms.send"]
	if got.Calls != 4 || got.WouldDeny != 3 || got.WildcardDeny != 1 {
		t.Fatalf("读回计数=%+v，期望 calls=4 would_deny=3 wildcard_deny=1", got)
	}

	// 新进程再观察一次后 flush：写的是绝对累计（4+1），不是本轮增量（1）。
	obs2.Observe(ctx, tooluse.RiskDecision{
		ToolName: "reach.sms.send", Level: tooluse.RiskHighWrite, Declared: true,
		WouldDeny: false, Reason: tooluse.RiskReasonGranted, At: now.Add(4 * time.Second),
	})
	if err := flushToolRiskStats(ctx, obs2, store); err != nil {
		t.Fatalf("第二次 flush 失败: %v", err)
	}
	obs3 := tooluse.NewMemoryRiskObserver(10)
	if _, err := loadToolRiskStats(ctx, obs3, store); err != nil {
		t.Fatalf("第二次 load 失败: %v", err)
	}
	if got := obs3.Counts()["reach.sms.send"]; got.Calls != 5 {
		t.Fatalf("绝对累计被写成增量：calls=%d，期望 5", got.Calls)
	}
}

func TestToolRiskStats_EmptyObserverDoesNotWrite(t *testing.T) {
	store := newFakeToolRiskStatStore()
	if err := flushToolRiskStats(context.Background(), tooluse.NewMemoryRiskObserver(10), store); err != nil {
		t.Fatalf("空 observer flush 应 no-op：%v", err)
	}
	if store.upserts != 0 {
		t.Errorf("空 observer 写了 %d 次库（会白白覆盖历史）", store.upserts)
	}
}

func TestStartToolRiskStatsPersistence_PeriodicFlushAndReload(t *testing.T) {
	store := newFakeToolRiskStatStore()
	obs := tooluse.NewMemoryRiskObserver(10)
	if !startToolRiskStatsPersistence(obs, store, 10*time.Millisecond) {
		t.Fatal("startToolRiskStatsPersistence 返回 false（空库也应视为已接上）")
	}
	t.Cleanup(CloseToolRiskStatsPersistence)
	if !ToolRiskStatsPersisted() {
		t.Fatal("接上后 ToolRiskStatsPersisted=false，报告的 observations_persisted 会谎报")
	}

	obs.Observe(context.Background(), tooluse.RiskDecision{
		ToolName: "reach.email.send", Level: tooluse.RiskHighWrite, Declared: true,
		WouldDeny: true, Reason: tooluse.RiskReasonWhitelistAbsent, At: time.Now(),
	})

	deadline := time.Now().Add(2 * time.Second)
	for store.raw(ToolRiskStatsKVKey) == "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if store.raw(ToolRiskStatsKVKey) == "" {
		t.Fatal("周期 flush 两个周期内没有写出数据")
	}

	reloaded := tooluse.NewMemoryRiskObserver(10)
	if _, err := loadToolRiskStats(context.Background(), reloaded, store); err != nil {
		t.Fatalf("load 失败: %v", err)
	}
	if got := reloaded.Counts()["reach.email.send"]; got.WouldDeny != 1 {
		t.Fatalf("周期 flush 后读回 would_deny=%d，期望 1", got.WouldDeny)
	}

	CloseToolRiskStatsPersistence()
	if ToolRiskStatsPersisted() {
		t.Error("Close 之后 ToolRiskStatsPersisted 仍为 true")
	}
}

func TestStartToolRiskStatsPersistence_LoadFailureDoesNotOverwrite(t *testing.T) {
	store := newFakeToolRiskStatStore()
	store.getErr = errors.New("db down")
	obs := tooluse.NewMemoryRiskObserver(10)
	if startToolRiskStatsPersistence(obs, store, time.Hour) {
		t.Fatal("读回失败却报告已接上：本轮 flush 会把历史覆盖成本进程的空计数")
	}
	if ToolRiskStatsPersisted() {
		t.Error("读回失败时 ToolRiskStatsPersisted 必须为 false")
	}
	if store.upserts != 0 {
		t.Errorf("读回失败后仍写了 %d 次库", store.upserts)
	}
}
