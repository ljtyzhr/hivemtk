package service

// reach_compliance_policy_test.go E1 合规策略判定器的纯单测（不碰 DB、不碰参数中心）。
//
// 覆盖的重点不是「能不能解析 JSON」，而是那些**只在特定组合下才错**的口径：
//   - 默认零行为变更（未配置 / enabled=false ⇒ 放行且原因 policy_disabled）
//   - 拒绝优先于延后（被地区禁止的渠道不该因为静默时段被排到 8 小时后照样发出去）
//   - 多处延后取最早释放点
//   - 日上限只在**成功委外之后**计数（被拒的请求烧配额会把自己锁死）
//   - 窗口越界不报红、但也不静默（视为未配置窗口，回落全局）

import (
	"context"
	"strings"
	"testing"
	"time"
)

func newTestCompliancePolicy(t *testing.T, raw string) *ConfigCompliancePolicy {
	t.Helper()
	p := NewConfigCompliancePolicy()
	p.SetPolicySourceForTest(func(context.Context) string { return raw })
	return p
}

// complianceFixedClock 固定时钟：2026-10-23 21:30 CST（落在多数地区静默窗口内）。
func complianceFixedClock() time.Time {
	return time.Date(2026, 10, 23, 21, 30, 0, 0, cstZone)
}

func TestParseCompliancePolicy_EmptyOrDisabled(t *testing.T) {
	for _, raw := range []string{"", "   ", "null", "{}", `{"enabled":false}`} {
		pol, err := ParseCompliancePolicy(raw)
		if err != nil {
			t.Fatalf("raw=%q 不应报错: %v", raw, err)
		}
		if pol.Enabled {
			t.Fatalf("raw=%q 不应视为启用", raw)
		}
	}
}

func TestParseCompliancePolicy_InvalidJSON(t *testing.T) {
	if _, err := ParseCompliancePolicy(`{"enabled":true,`); err == nil {
		t.Fatal("非法 JSON 必须报错（否则坏文档会静默变成「未启用」而没人知道）")
	}
}

func TestValidateCompliancePolicy_StructuralErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "日上限为负",
			raw:  `{"enabled":true,"regions":{"US":{"daily_limit":-1}}}`,
			want: "daily_limit",
		},
		{
			name: "渠道名单含空串",
			raw:  `{"enabled":true,"regions":{"US":{"denied_channels":[""]}}}`,
			want: "空串",
		},
		{
			name: "地区空键",
			raw:  `{"enabled":true,"regions":{" ":{}}}`,
			want: "空键",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseCompliancePolicy(c.raw)
			if err == nil {
				t.Fatalf("应报错（%s）", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("错误串应含 %q，实际 %v", c.want, err)
			}
		})
	}
}

func TestParseCompliancePolicy_NormalizesKeysAndChannels(t *testing.T) {
	pol, err := ParseCompliancePolicy(`{
		"enabled": true,
		"default_region": " cn ",
		"regions": {"us": {"denied_channels": [" SMS ", "Email"]}},
		"industries": {"MEDICAL": {"allowed_channels": ["WeCom"]}}
	}`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if pol.DefaultRegion != "CN" {
		t.Errorf("default_region 应规范为大写 CN，实际 %q", pol.DefaultRegion)
	}
	if _, ok := pol.Regions["US"]; !ok {
		t.Errorf("地区键应规范为大写 US，实际 keys=%v", pol.Regions)
	}
	got := pol.Regions["US"].DeniedChannels
	if len(got) != 2 || got[0] != "sms" || got[1] != "email" {
		t.Errorf("渠道名应规范为小写去空格，实际 %v", got)
	}
	if _, ok := pol.Industries["medical"]; !ok {
		t.Errorf("行业键应规范为小写 medical，实际 keys=%v", pol.Industries)
	}
	if got := pol.Industries["medical"].AllowedChannels; len(got) != 1 || got[0] != "wecom" {
		t.Errorf("行业白名单应规范为 wecom，实际 %v", got)
	}
}

func TestParseCompliancePolicy_OutOfRangeQuietHourFallsBackToGlobalWindow(t *testing.T) {
	pol, err := ParseCompliancePolicy(`{"enabled":true,"regions":{"US":{"quiet_hours_start":220,"quiet_hours_end":10}}}`)
	if err != nil {
		t.Fatalf("窗口越界不应让整份文档不可用（那会把一个手误升级成全渠道停机）: %v", err)
	}
	rule := pol.Regions["US"]
	if rule.QuietHoursStart != nil {
		t.Fatalf("越界起点应被清零（视为未配置），实际 %v", *rule.QuietHoursStart)
	}
	// 起点清零 ⇒ 回落全局起点；终点仍是配置值。
	start, end, ok := complianceQuietWindow(rule)
	if !ok {
		t.Fatal("终点仍配着，窗口应视为已配置")
	}
	if start != quietHoursStartHour || end != 10 {
		t.Errorf("窗口应为(全局起点=%d, 10)，实际(%d, %d)", quietHoursStartHour, start, end)
	}
}

func TestDecide_DisabledPolicyAllowsEverything(t *testing.T) {
	p := newTestCompliancePolicy(t, "")
	dec := p.Decide(context.Background(), ComplianceInput{Channel: "sms", CustomerID: "c1"})
	if !dec.Allowed || dec.Deferred {
		t.Fatalf("未配置策略必须放行，实际 %+v", dec)
	}
	if dec.Reason != ComplianceReasonDisabled {
		t.Errorf("原因码应为 %s，实际 %s", ComplianceReasonDisabled, dec.Reason)
	}
}

func TestDecide_RegionQuietHoursDefers(t *testing.T) {
	p := newTestCompliancePolicy(t, `{
		"enabled": true,
		"regions": {"US": {"quiet_hours_start": 20, "quiet_hours_end": 10}}
	}`)
	p.SetClock(complianceFixedClock) // 21:30 CST

	dec := p.Decide(context.Background(), ComplianceInput{Channel: "email", Region: "us", CustomerID: "c1"})
	if !dec.Allowed || !dec.Deferred {
		t.Fatalf("静默窗口内应判延后，实际 %+v", dec)
	}
	if dec.Reason != ComplianceReasonQuietHours {
		t.Errorf("原因码应为 %s，实际 %s", ComplianceReasonQuietHours, dec.Reason)
	}
	// 21:30 已过 10:00 ⇒ 释放点是次日 10:00。
	want := time.Date(2026, 10, 24, 10, 0, 0, 0, cstZone)
	if !dec.SendAt.Equal(want) {
		t.Errorf("释放点应为 %s，实际 %s", want, dec.SendAt)
	}
	if dec.Region != "US" {
		t.Errorf("地区应规范为大写 US，实际 %q", dec.Region)
	}
}

func TestDecide_RegionOutsideQuietHoursAllows(t *testing.T) {
	p := newTestCompliancePolicy(t, `{"enabled":true,"regions":{"US":{"quiet_hours_start":20,"quiet_hours_end":10}}}`)
	p.SetClock(func() time.Time { return time.Date(2026, 10, 23, 11, 0, 0, 0, cstZone) })

	dec := p.Decide(context.Background(), ComplianceInput{Channel: "email", Region: "US", CustomerID: "c1"})
	if !dec.Allowed || dec.Deferred {
		t.Fatalf("窗口外应放行，实际 %+v", dec)
	}
	if dec.Reason != ComplianceReasonAllowed {
		t.Errorf("原因码应为 %s，实际 %s", ComplianceReasonAllowed, dec.Reason)
	}
}

func TestDecide_ChannelDeniedBeatsQuietHours(t *testing.T) {
	// 21:30 落在静默窗口内，但 sms 被地区明令禁止：必须直接拒，不许进延迟队列。
	p := newTestCompliancePolicy(t, `{"enabled":true,"regions":{"US":{
		"denied_channels":["sms"],"quiet_hours_start":20,"quiet_hours_end":10}}}`)
	p.SetClock(complianceFixedClock)

	dec := p.Decide(context.Background(), ComplianceInput{Channel: "sms", Region: "US", CustomerID: "c1"})
	if dec.Allowed || dec.Deferred {
		t.Fatalf("被禁止的渠道必须直接拒绝（不能延后后照样发），实际 %+v", dec)
	}
	if dec.Reason != ComplianceReasonChannelDenied {
		t.Errorf("原因码应为 %s，实际 %s", ComplianceReasonChannelDenied, dec.Reason)
	}
	if dec.PolicyScope != "region:US" {
		t.Errorf("规则域应为 region:US，实际 %q", dec.PolicyScope)
	}
}

func TestDecide_IndustryAllowedChannelsWhitelist(t *testing.T) {
	p := newTestCompliancePolicy(t, `{"enabled":true,"industries":{"medical":{"allowed_channels":["wecom"]}}}`)
	p.SetClock(func() time.Time { return time.Date(2026, 10, 23, 11, 0, 0, 0, cstZone) })

	denied := p.Decide(context.Background(), ComplianceInput{Channel: "sms", Industry: "medical", CustomerID: "c1"})
	if denied.Allowed {
		t.Fatalf("白名单外渠道应拒绝，实际 %+v", denied)
	}
	if denied.Reason != ComplianceReasonChannelNotAllowed {
		t.Errorf("原因码应为 %s，实际 %s", ComplianceReasonChannelNotAllowed, denied.Reason)
	}

	allowed := p.Decide(context.Background(), ComplianceInput{Channel: "wecom", Industry: "MEDICAL", CustomerID: "c1"})
	if !allowed.Allowed {
		t.Fatalf("白名单内渠道应放行，实际 %+v", allowed)
	}
}

func TestDecide_RequireOptIn(t *testing.T) {
	p := newTestCompliancePolicy(t, `{"enabled":true,"industries":{"medical":{"require_opt_in":true}}}`)
	p.SetClock(func() time.Time { return time.Date(2026, 10, 23, 11, 0, 0, 0, cstZone) })

	noOptIn := p.Decide(context.Background(), ComplianceInput{Channel: "wecom", Industry: "medical", CustomerID: "c1"})
	if noOptIn.Allowed || noOptIn.Reason != ComplianceReasonOptInRequired {
		t.Fatalf("缺同意应拒绝，原因 opt_in_required，实际 %+v", noOptIn)
	}

	withOptIn := p.Decide(context.Background(), ComplianceInput{Channel: "wecom", Industry: "medical", CustomerID: "c1", OptIn: true})
	if !withOptIn.Allowed {
		t.Fatalf("有同意应放行，实际 %+v", withOptIn)
	}
}

func TestDecide_DailyLimitCountsOnlySuccessfulSends(t *testing.T) {
	p := newTestCompliancePolicy(t, `{"enabled":true,"regions":{"US":{"daily_limit":1}}}`)
	p.SetClock(func() time.Time { return time.Date(2026, 10, 23, 11, 0, 0, 0, cstZone) })
	ctx := context.Background()
	in := ComplianceInput{Channel: "email", Region: "US", CustomerID: "c1"}

	if dec := p.Decide(ctx, in); !dec.Allowed {
		t.Fatalf("首次应放行，实际 %+v", dec)
	}
	// 模拟「被拒/未发出」的请求：不调用 RecordSent ⇒ 计数不变 ⇒ 仍然放行。
	if dec := p.Decide(ctx, in); !dec.Allowed {
		t.Fatalf("未成功外发不应消耗配额，实际 %+v", dec)
	}

	p.RecordSentInput(ctx, in)
	if dec := p.Decide(ctx, in); dec.Allowed {
		t.Fatalf("已达上限应拒绝，实际 %+v", dec)
	} else if dec.Reason != ComplianceReasonDailyLimit {
		t.Errorf("原因码应为 %s，实际 %s", ComplianceReasonDailyLimit, dec.Reason)
	}
}

func TestDecide_DailyLimitIsPerDay(t *testing.T) {
	p := newTestCompliancePolicy(t, `{"enabled":true,"regions":{"US":{"daily_limit":1}}}`)
	ctx := context.Background()
	in := ComplianceInput{Channel: "email", Region: "US", CustomerID: "c1"}

	p.SetClock(func() time.Time { return time.Date(2026, 10, 23, 11, 0, 0, 0, cstZone) })
	p.RecordSentInput(ctx, in)
	if dec := p.Decide(ctx, in); dec.Allowed {
		t.Fatalf("当日已达上限应拒绝，实际 %+v", dec)
	}

	p.SetClock(func() time.Time { return time.Date(2026, 10, 24, 11, 0, 0, 0, cstZone) })
	if dec := p.Decide(ctx, in); !dec.Allowed {
		t.Fatalf("次日应恢复额度，实际 %+v", dec)
	}
}

func TestDecide_UnknownRegion(t *testing.T) {
	t.Run("默认放行并留痕", func(t *testing.T) {
		p := newTestCompliancePolicy(t, `{"enabled":true,"regions":{"US":{}}}`)
		p.SetClock(func() time.Time { return time.Date(2026, 10, 23, 11, 0, 0, 0, cstZone) })
		dec := p.Decide(context.Background(), ComplianceInput{Channel: "sms", Region: "JP", CustomerID: "c1"})
		if !dec.Allowed {
			t.Fatalf("unknown_region_deny=false 时应放行，实际 %+v", dec)
		}
		if dec.Reason != ComplianceReasonRegionUnruled {
			t.Errorf("应留痕 region_unruled，实际 %s", dec.Reason)
		}
	})
	t.Run("显式 fail-closed", func(t *testing.T) {
		p := newTestCompliancePolicy(t, `{"enabled":true,"unknown_region_deny":true,"regions":{"US":{}}}`)
		dec := p.Decide(context.Background(), ComplianceInput{Channel: "sms", Region: "JP", CustomerID: "c1"})
		if dec.Allowed || dec.Reason != ComplianceReasonRegionUnruled {
			t.Fatalf("unknown_region_deny=true 时应拒绝，实际 %+v", dec)
		}
	})
}

func TestDecide_DefaultRegionFallback(t *testing.T) {
	p := newTestCompliancePolicy(t, `{"enabled":true,"default_region":"US","regions":{"US":{"denied_channels":["sms"]}}}`)
	dec := p.Decide(context.Background(), ComplianceInput{Channel: "sms", CustomerID: "c1"})
	if dec.Allowed {
		t.Fatal("请求未带地区时应回落 default_region=US，并按该规则拒绝 sms")
	}
	if dec.Region != "US" {
		t.Errorf("生效地区应为 US，实际 %q", dec.Region)
	}
}

func TestDecide_RegionAndIndustryStackedAnyDenyWins(t *testing.T) {
	p := newTestCompliancePolicy(t, `{"enabled":true,
		"regions":{"US":{}},
		"industries":{"medical":{"denied_channels":["sms"]}}}`)
	dec := p.Decide(context.Background(), ComplianceInput{Channel: "sms", Region: "US", Industry: "medical", CustomerID: "c1"})
	if dec.Allowed {
		t.Fatalf("地区放行但行业禁止 ⇒ 必须拒绝，实际 %+v", dec)
	}
	if dec.PolicyScope != "industry:medical" {
		t.Errorf("规则域应为 industry:medical，实际 %q", dec.PolicyScope)
	}
}

func TestDecide_EarliestDeferWins(t *testing.T) {
	// 地区窗口 20–9（释放 9:00），行业窗口 20–8（释放 8:00）⇒ 取最早 8:00。
	p := newTestCompliancePolicy(t, `{"enabled":true,
		"regions":{"US":{"quiet_hours_start":20,"quiet_hours_end":9}},
		"industries":{"medical":{"quiet_hours_start":20,"quiet_hours_end":8}}}`)
	p.SetClock(complianceFixedClock) // 21:30

	dec := p.Decide(context.Background(), ComplianceInput{Channel: "email", Region: "US", Industry: "medical", CustomerID: "c1"})
	if !dec.Deferred {
		t.Fatalf("应判延后，实际 %+v", dec)
	}
	want := time.Date(2026, 10, 24, 8, 0, 0, 0, cstZone)
	if !dec.SendAt.Equal(want) {
		t.Errorf("多处延后应取最早释放点 %s，实际 %s", want, dec.SendAt)
	}
}

func TestCompliancePolicy_BadDocumentDegradesToDisabledWithoutFlood(t *testing.T) {
	p := newTestCompliancePolicy(t, `{"enabled":true,`)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		dec := p.Decide(ctx, ComplianceInput{Channel: "sms", CustomerID: "c1"})
		if !dec.Allowed {
			t.Fatalf("坏文档按未启用处理（放行），实际 %+v", dec)
		}
	}
	// 同一份坏原文只 ERROR 一次：门是「喊一次」，不是「每次外发都刷屏」。
	if p.lastBadRaw != `{"enabled":true,` {
		t.Errorf("应记住已喊过的坏原文，实际 %q", p.lastBadRaw)
	}
}

func TestCompliancePolicy_CacheInvalidatedOnParamChange(t *testing.T) {
	raw := `{"enabled":true,"regions":{"US":{"denied_channels":["sms"]}}}`
	p := NewConfigCompliancePolicy()
	p.SetPolicySourceForTest(func(context.Context) string { return raw })

	ctx := context.Background()
	if dec := p.Decide(ctx, ComplianceInput{Channel: "sms", Region: "US", CustomerID: "c1"}); dec.Allowed {
		t.Fatal("sms 应被拒")
	}
	raw = `{"enabled":false}`
	if dec := p.Decide(ctx, ComplianceInput{Channel: "sms", Region: "US", CustomerID: "c1"}); !dec.Allowed {
		t.Fatal("参数变更（原文变化）后缓存必须失效，否则运维改完策略 60s 都不生效")
	}
}

func TestMemoryComplianceUsageCounter_ResetsOnOverflow(t *testing.T) {
	c := NewMemoryComplianceUsageCounter()
	if got := c.Count(context.Background(), "k", "2026-10-23"); got != 0 {
		t.Fatalf("初始应为 0，实际 %d", got)
	}
	c.Incr(context.Background(), "k", "2026-10-23")
	if got := c.Count(context.Background(), "k", "2026-10-23"); got != 1 {
		t.Fatalf("应为 1，实际 %d", got)
	}
	if got := c.Count(context.Background(), "k", "2026-10-24"); got != 0 {
		t.Fatalf("不同自然日应独立计数，实际 %d", got)
	}
}

func TestComplianceDecision_DecisionString(t *testing.T) {
	cases := []struct {
		d    ComplianceDecision
		want string
	}{
		{ComplianceDecision{Allowed: true}, ComplianceDecisionAllow},
		{ComplianceDecision{Allowed: false}, ComplianceDecisionDeny},
		{ComplianceDecision{Allowed: true, Deferred: true}, ComplianceDecisionDefer},
	}
	for _, c := range cases {
		if got := c.d.Decision(); got != c.want {
			t.Errorf("%+v 的判定动作应为 %s，实际 %s", c.d, c.want, got)
		}
	}
}

func TestEvaluate_ReadsMetadata(t *testing.T) {
	p := newTestCompliancePolicy(t, `{"enabled":true,"industries":{"medical":{"require_opt_in":true}}}`)
	req := &ReachSendRequest{
		Channel:  "wecom",
		Metadata: map[string]string{ReachMetaIndustry: "medical"},
	}
	if dec := p.Evaluate(context.Background(), req); dec.Allowed {
		t.Fatal("metadata.industry=medical 且无 opt_in 应拒绝")
	}
	req.Metadata[ReachMetaConsent] = "true"
	if dec := p.Evaluate(context.Background(), req); !dec.Allowed {
		t.Fatalf("metadata.consent=true 应视为已同意，实际 %+v", dec)
	}
}

func TestCopyComplianceBatch_KeepsDecisionColumns(t *testing.T) {
	// 漏拷判定列 = 审计表只剩 channel/recipient，事后答不出「为什么被拒」。
	batch := []*ReachComplianceLog{{
		Channel: "sms", RecipientID: "r1",
		Decision: ComplianceDecisionDeny, Reason: ComplianceReasonChannelNotAllowed,
		Region: "US", Industry: "medical", PolicyScope: "industry:medical",
	}}
	out := copyComplianceBatch(batch)
	if len(out) != 1 {
		t.Fatalf("应转出 1 行，实际 %d", len(out))
	}
	got := out[0]
	if got.Decision != ComplianceDecisionDeny || got.Reason != ComplianceReasonChannelNotAllowed ||
		got.Region != "US" || got.Industry != "medical" || got.PolicyScope != "industry:medical" {
		t.Fatalf("判定列未完整透传: %+v", got)
	}
}
