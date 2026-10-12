package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/model"
)

// 能力面授予缓存的行为用例：判据都打在**注入的假 provider** 上，
// 不碰库（真库那条腿在 internal/router 的常驻门里）。

func withProvider(t *testing.T, p MenuGrantProvider) {
	t.Helper()
	prev := menuGrantProvider
	SetMenuGrantProvider(p)
	t.Cleanup(func() {
		SetMenuGrantProvider(prev)
	})
}

func TestCapabilityGrant_CacheTTLAndStaleWhileError(t *testing.T) {
	var calls int
	var failing bool
	now := time.Unix(1700000000, 0)
	prevNow := menuGrantNow
	menuGrantNow = func() time.Time { return now }
	t.Cleanup(func() { menuGrantNow = prevNow })

	withProvider(t, func(_ context.Context, role string) (map[string]struct{}, error) {
		calls++
		if failing {
			return nil, errors.New("db down")
		}
		keys := make(map[string]struct{})
		if role == "staff" {
			keys["systemFeatureFlags"] = struct{}{}
		}
		return keys, nil
	})

	for i := 0; i < 3; i++ {
		if _, ok, err := grantedMenuKeys(context.Background(), "staff"); err != nil || !ok {
			t.Fatalf("第 %d 次取授予失败 ok=%v err=%v", i+1, ok, err)
		}
	}
	if calls != 1 {
		t.Errorf("TTL 内查了 %d 次库，应为 1 次（缓存没生效）", calls)
	}

	now = now.Add(menuGrantTTL + time.Second)
	if _, ok, err := grantedMenuKeys(context.Background(), "staff"); err != nil || !ok {
		t.Fatalf("TTL 到期后取授予失败: %v", err)
	}
	if calls != 2 {
		t.Errorf("TTL 到期后没重查（calls=%d，应为 2）", calls)
	}

	// stale-while-error：查询报错时沿用旧值，库抖动不该把坐席全数拦在门外。
	failing = true
	now = now.Add(menuGrantTTL + time.Second)
	keys, ok, err := grantedMenuKeys(context.Background(), "staff")
	if err == nil {
		t.Error("provider 已改为报错，grantedMenuKeys 却报成功：stale 分支没走到")
	}
	if !ok || len(keys) != 1 {
		t.Errorf("查询失败时应沿用旧读数，实得 ok=%v keys=%d", ok, len(keys))
	}
}

func TestCapabilityGrant_NoProviderFailsClosed(t *testing.T) {
	withProvider(t, nil)
	// 表内路径 + 没有授予查询＝装配缺件，必须判"拦"，不能静默放行。
	// 用表内一条真实 pattern 的语义：这里直接测 grantedMenuKeys 的三值契约。
	_, ok, err := grantedMenuKeys(context.Background(), "staff")
	if ok || err != nil {
		t.Errorf("provider 为 nil 时应返回 (false,nil) 让上层按无授予拦，实得 ok=%v err=%v", ok, err)
	}
}

func TestCapabilityInvalidateDropsEntry(t *testing.T) {
	var calls int
	withProvider(t, func(_ context.Context, _ string) (map[string]struct{}, error) {
		calls++
		return map[string]struct{}{}, nil
	})
	if _, _, err := grantedMenuKeys(context.Background(), "customer_service"); err != nil {
		t.Fatalf("取授予失败: %v", err)
	}
	InvalidateMenuGrants("customer_service")
	if _, _, err := grantedMenuKeys(context.Background(), "customer_service"); err != nil {
		t.Fatalf("撤缓存后取授予失败: %v", err)
	}
	if calls != 2 {
		t.Errorf("InvalidateMenuGrants 没让下一次重新查库（calls=%d，应为 2）⇒ 界面改完授权要等 TTL 才生效", calls)
	}
}

// TestCapabilityMatcherEquivalenceWithRoutePolicy 两处段匹配必须同形。
//
// model 侧自己实现了一份 matchSegments（反向 import 会成环），所以等价性只能靠用例钉。
// 表里刻意不出现 `**`：那是仅超管表独有的尾段通配，能力面规则用了它两处就会分叉。
func TestCapabilityMatcherEquivalenceWithRoutePolicy(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/api/glossaries", "/api/glossaries", true},
		{"/api/glossaries", "/api/glossaries/", true}, // 两侧都 Trim 斜杠
		{"/api/glossaries/*", "/api/glossaries/12", true},
		{"/api/glossaries/*", "/api/glossaries", false},     // `*` 只吃恰好一段
		{"/api/glossaries/*", "/api/glossaries/1/2", false}, // 段数不等
		{"/api/a/*/c", "/api/b/c", false},
		{"/api/exact", "/api/other", false},
	}
	for _, c := range cases {
		if got := model.MatchCapabilityPattern(c.pattern, c.path); got != c.want {
			t.Errorf("model.MatchCapabilityPattern(%q,%q)=%v，应为 %v", c.pattern, c.path, got, c.want)
		}
		if got := matchSegments(c.pattern, c.path); got != c.want {
			t.Errorf("middleware.matchSegments(%q,%q)=%v，与 model 侧不同形（两处判据分叉）",
				c.pattern, c.path, got)
		}
	}
}
