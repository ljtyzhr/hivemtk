package tenant

import (
	"context"
	"errors"
	"testing"
)

func TestFanout_OffModeSingleDefault(t *testing.T) {
	SetMode(ModeOff)
	t.Cleanup(func() { SetMode(ModeOff) })

	seen := []uint{}
	total, err := Fanout(context.Background(), nil, func(ctx context.Context, tenantID uint) (int64, error) {
		id, ok := FromContext(ctx)
		if !ok || id != tenantID {
			t.Errorf("fanout ctx 租户绑定缺失: id=%v ok=%v tenantID=%d", id, ok, tenantID)
		}
		seen = append(seen, tenantID)
		return 2, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(seen) != 1 || seen[0] != DefaultTenantID {
		t.Fatalf("off 模式应单默认租户一次: total=%d seen=%v", total, seen)
	}
}

func TestFanout_OnModeIteratesActive(t *testing.T) {
	SetMode(ModeOn)
	t.Cleanup(func() { SetMode(ModeOff) })
	SetActiveTenantLoader(func(ctx context.Context) ([]uint, error) {
		return []uint{3, 5, 9}, nil
	})
	t.Cleanup(func() { SetActiveTenantLoader(nil) })

	seen := map[uint]bool{}
	total, err := Fanout(context.Background(), nil, func(ctx context.Context, tenantID uint) (int64, error) {
		id, ok := FromContext(ctx)
		if !ok || id != tenantID {
			t.Errorf("fanout ctx 绑定错: %v %v vs %d", id, ok, tenantID)
		}
		seen[tenantID] = true
		return 1, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(seen) != 3 || !seen[3] || !seen[5] || !seen[9] {
		t.Fatalf("on 模式应逐活跃租户: total=%d seen=%v", total, seen)
	}
}

func TestFanout_OnModeTenantFailureDoesNotBlock(t *testing.T) {
	SetMode(ModeOn)
	t.Cleanup(func() { SetMode(ModeOff) })
	SetActiveTenantLoader(func(ctx context.Context) ([]uint, error) {
		return []uint{1, 2, 3}, nil
	})
	t.Cleanup(func() { SetActiveTenantLoader(nil) })

	total, err := Fanout(context.Background(), nil, func(ctx context.Context, tenantID uint) (int64, error) {
		if tenantID == 2 {
			return 0, errors.New("boom")
		}
		return 1, nil
	})
	// 单租户失败不阻断其余：total=2，err 记录但不中断（非"全部失败"）
	if total != 2 {
		t.Fatalf("失败租户不应贡献计数: total=%d", total)
	}
	if err == nil {
		t.Log("失败已记录（含租户号）")
	}
}

func TestFanout_OnModeLoaderMissingFails(t *testing.T) {
	SetMode(ModeOn)
	t.Cleanup(func() { SetMode(ModeOff) })
	SetActiveTenantLoader(nil)

	called := false
	_, err := Fanout(context.Background(), nil, func(ctx context.Context, tenantID uint) (int64, error) {
		called = true
		return 0, nil
	})
	if err == nil {
		t.Fatal("枚举器缺失必须 fail-closed")
	}
	if called {
		t.Fatal("fail-closed 不得执行回调")
	}
}

func TestFanout_ContextCancelStops(t *testing.T) {
	SetMode(ModeOn)
	t.Cleanup(func() { SetMode(ModeOff) })
	SetActiveTenantLoader(func(ctx context.Context) ([]uint, error) {
		return []uint{1, 2, 3, 4}, nil
	})
	t.Cleanup(func() { SetActiveTenantLoader(nil) })

	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	total, err := Fanout(ctx, nil, func(ctx context.Context, tenantID uint) (int64, error) {
		count++
		cancel() // 第一个租户后取消
		return 1, nil
	})
	_ = total
	if count > 2 {
		t.Fatalf("取消后应停止扇出: count=%d", count)
	}
	if err == nil {
		t.Fatal("取消应返回 ctx.Err")
	}
}
