package tenant

import (
	"context"
	"strings"
	"testing"
)

func TestRedisKey_OffModeByteIdentical(t *testing.T) {
	SetMode(ModeOff)
	t.Cleanup(func() { SetMode(ModeOff) })

	ctx := WithTenantID(context.Background(), 42)
	// off 模式：带不带租户 ctx，键形都必须与现状逐字节一致
	for _, key := range []string{
		"jwt:blacklist:abc123",
		"mtk:jwt:revoke:5",
		"mtk:ratelimit:1.2.3.4:202610171200",
		"some:session:key",
	} {
		if got := RedisKey(ctx, key); got != key {
			t.Errorf("off 模式键形必须不变: RedisKey(%q) = %q", key, got)
		}
		if got := RedisKey(context.Background(), key); got != key {
			t.Errorf("off 模式无租户键形必须不变: %q -> %q", key, got)
		}
		if got := RedisKeyWithTenant(42, key); got != key {
			t.Errorf("off 模式 RedisKeyWithTenant 键形必须不变: %q", got)
		}
	}
	if got := RedisKey(ctx, ""); got != "" {
		t.Errorf("空键必须原样返回, got %q", got)
	}
}

func TestRedisKey_OnModePrefixed(t *testing.T) {
	SetMode(ModeOn)
	t.Cleanup(func() { SetMode(ModeOff) })

	ctx := WithTenantID(context.Background(), 42)
	got := RedisKey(ctx, "mtk:jwt:revoke:5")
	if got != "t42:mtk:jwt:revoke:5" {
		t.Fatalf("on 模式键应带租户前缀, got %q", got)
	}

	// 无租户 ctx：裸键（全局语义）
	bare := RedisKey(context.Background(), "mtk:jwt:revoke:5")
	if bare != "mtk:jwt:revoke:5" {
		t.Fatalf("on 模式无租户 ctx 应裸键, got %q", bare)
	}

	// 显式租户维度构造
	if got := RedisKeyWithTenant(7, "quota:daily"); got != "t7:quota:daily" {
		t.Fatalf("RedisKeyWithTenant 异常: %q", got)
	}
	if got := RedisKeyWithTenant(0, "quota:daily"); got != "quota:daily" {
		t.Fatalf("tenant 0 必须裸键, got %q", got)
	}

	// 不同租户键互不相同
	a := RedisKey(WithTenantID(context.Background(), 1), "k")
	b := RedisKey(WithTenantID(context.Background(), 2), "k")
	if a == b || strings.HasPrefix(a, "t2:") || strings.HasPrefix(b, "t1:") {
		t.Fatalf("不同租户键必须分域: %q vs %q", a, b)
	}
}
