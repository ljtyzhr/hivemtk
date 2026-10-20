package reporting

import (
	"context"
	"testing"
)

// TestDefaultWindowDays_NoProvider 未注入 provider 时回落编译期兜底 30（行为零变更）。
func TestDefaultWindowDays_NoProvider(t *testing.T) {
	SetDefaultWindowDaysProvider(nil)
	t.Cleanup(func() { SetDefaultWindowDaysProvider(nil) })
	if got := DefaultWindowDays(context.Background()); got != 30 {
		t.Errorf("无 provider 时 = %d, 期望 30", got)
	}
}

// TestDefaultWindowDays_ProviderHotRead 注入后每次调用都走 provider（热读，不是装配期快照）。
func TestDefaultWindowDays_ProviderHotRead(t *testing.T) {
	calls := 0
	SetDefaultWindowDaysProvider(func(ctx context.Context) int {
		calls++
		return 7
	})
	t.Cleanup(func() { SetDefaultWindowDaysProvider(nil) })
	if got := DefaultWindowDays(context.Background()); got != 7 {
		t.Errorf("注入 7 后 = %d, 期望 7", got)
	}
	if got := DefaultWindowDays(context.Background()); got != 7 {
		t.Errorf("第二次读 = %d, 期望 7（应每次热读）", got)
	}
	if calls != 2 {
		t.Errorf("provider 被调 %d 次, 期望 2（证明不是装配期快照）", calls)
	}
}

// TestDefaultWindowDays_NonPositiveFallsBack provider 返回非正值时回落 30（防把窗口写成 0/负数）。
func TestDefaultWindowDays_NonPositiveFallsBack(t *testing.T) {
	for _, v := range []int{0, -5} {
		SetDefaultWindowDaysProvider(func(ctx context.Context) int { return v })
		if got := DefaultWindowDays(context.Background()); got != 30 {
			t.Errorf("provider 返回 %d 时 = %d, 期望回落 30", v, got)
		}
	}
	SetDefaultWindowDaysProvider(nil)
}
