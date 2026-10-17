package llm

import "testing"

// LLM 计价口径的判据测试。
//
// 需求原文：「商户使用平台端 llM 时，实时扣减剩余 token 数，
// **输入和输出分别计价**以及扣费」。
//
// 旧实现 splitCost 是「总价按 token 比例劈两半」—— 数学上等价于输入输出同价，
// 表达不出「输出贵」这个事实。下面这组用例把「分别计价」与「按比例拆分」
// 区分开：同一组 token、两种单价下，两种口径会给出**不同的**结果。

func TestComputeCostSeparatePricing(t *testing.T) {
	p := ProviderConfig{
		CostPer1k:       1, // 老列填着也不该影响分列口径
		CostPer1kInput:  0.5,
		CostPer1kOutput: 3,
	}
	// 1000 输入 + 1000 输出
	total, in, out := computeCost(p, 1000, 1000)
	if in != 0.5 || out != 3 {
		t.Fatalf("分别计价不符: in=%v out=%v", in, out)
	}
	if total != 3.5 {
		t.Fatalf("合计应为 3.5，实际 %v", total)
	}
	// 反证：若退化成「按比例拆分」，两部分必然相等 —— 这正是旧实现的形状
	if in == out {
		t.Fatal("输入输出单价不同，结果却相等：实现可能退化成了按比例拆分总价")
	}
}

func TestComputeCostFallsBackToSinglePrice(t *testing.T) {
	// 只配了老列 → 两部分回落成同价，**存量成本统计不变**
	p := ProviderConfig{CostPer1k: 2}
	_, in, out := computeCost(p, 1000, 1000)
	if in != 2 || out != 2 {
		t.Fatalf("单一单价回落失效: in=%v out=%v（存量 provider 的成本会被改写）", in, out)
	}
	// 一列配了、另一列没配 → 没配的那列回落老值
	p2 := ProviderConfig{CostPer1k: 2, CostPer1kOutput: 5}
	_, in2, out2 := computeCost(p2, 1000, 1000)
	if in2 != 2 || out2 != 5 {
		t.Fatalf("部分配置回落失效: in=%v out=%v", in2, out2)
	}
}

func TestComputeCostZeroAndBoundary(t *testing.T) {
	p := ProviderConfig{CostPer1kInput: 1, CostPer1kOutput: 4}
	if total, _, _ := computeCost(p, 0, 0); total != 0 {
		t.Errorf("零 token 应计 0，实际 %v", total)
	}
	if total, _, _ := computeCost(p, -5, -5); total != 0 {
		t.Errorf("负 token（估算失败时可能出现）不应计费，实际 %v", total)
	}
	total, in, out := computeCost(p, 500, 250)
	if in != 0.5 || out != 1 || total != 1.5 {
		t.Errorf("小数量级计价错误: total=%v in=%v out=%v", total, in, out)
	}
}

func TestProviderSourceDefaultsToSelf(t *testing.T) {
	// 漏填 Source 的 provider 绝大多数是用户自己加的，落成 platform 会凭空记账
	if got := (ProviderConfig{}).ProviderSource(); got != "self" {
		t.Errorf("空 Source 应回落 self，实际 %q", got)
	}
	if (ProviderConfig{}).IsPlatformProvider() {
		t.Error("空 Source 不得判为平台 provider")
	}
	if !(ProviderConfig{Source: "platform"}).IsPlatformProvider() {
		t.Error("source=platform 应判为平台 provider")
	}
}

func TestHasSeparatePricing(t *testing.T) {
	cases := []struct {
		name string
		p    ProviderConfig
		want bool
	}{
		{"分列且不同", ProviderConfig{CostPer1kInput: 1, CostPer1kOutput: 4}, true},
		{"分列但相同", ProviderConfig{CostPer1kInput: 2, CostPer1kOutput: 2}, false},
		{"只配老列", ProviderConfig{CostPer1k: 2}, false},
		{"分列+老列且分列不同", ProviderConfig{CostPer1k: 9, CostPer1kInput: 1, CostPer1kOutput: 4}, true},
	}
	for _, c := range cases {
		if got := c.p.HasSeparatePricing(); got != c.want {
			t.Errorf("%s: HasSeparatePricing=%v，期望 %v", c.name, got, c.want)
		}
	}
}

// TestSplitCostLegacyStillRatio 用于把旧行为钉住：splitCost 仍服务历史数据展示，
// 但**不能**再被当成计费依据（computeCost 才是）。这条测试的作用是
// 「有人误以为 splitCost 已改成分别计价时，会立刻看到它仍是比例拆分」。
func TestSplitCostLegacyStillRatio(t *testing.T) {
	total := 10.0
	prompt, completion := splitCost(total, 8000, 2000, 10000)
	if prompt != 8.0 || completion != 2.0 {
		t.Fatalf("splitCost 语义应保持比例拆分（历史数据展示依赖它）: prompt=%v completion=%v", prompt, completion)
	}
}
