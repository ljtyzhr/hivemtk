package model

import (
	"testing"
	"time"
)

// TestTenantModuleUsableByBillingCycle 4 种计费周期的 Usable 判定（003 T004 验收）。
//
// 判定载体是 ExpiresAt 而不是周期字符串本身（002 ADR：放同一枚举复用同一套
// 过期判定，不新增第二套周期字段）：
//   - once    ：买断，永不写 ExpiresAt → 恒 usable
//   - metered ：钱包计量，不设 ExpiresAt → 恒 usable（扣费在钱包层）
//   - yearly/monthly：写 ExpiresAt，未来 usable、过期拦
func TestTenantModuleUsableByBillingCycle(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name       string
		row        *TenantModule
		wantUsable bool
	}{
		{"once_买断无到期日", &TenantModule{Status: TenantModuleActive}, true},
		{"metered_计量无到期日", &TenantModule{Status: TenantModuleActive}, true},
		{"yearly_未到期", &TenantModule{Status: TenantModuleActive, ExpiresAt: &future}, true},
		{"yearly_已过期", &TenantModule{Status: TenantModuleActive, ExpiresAt: &past}, false},
		{"monthly_未到期", &TenantModule{Status: TenantModuleActive, ExpiresAt: &future}, true},
		{"monthly_已过期", &TenantModule{Status: TenantModuleActive, ExpiresAt: &past}, false},
		{"disabled_禁用态不论周期", &TenantModule{Status: TenantModuleDisabled}, false},
	}
	for _, c := range cases {
		if got := TenantModuleUsable(c.row, now); got != c.wantUsable {
			t.Errorf("%s: usable=%v，期望 %v", c.name, got, c.wantUsable)
		}
	}
}
