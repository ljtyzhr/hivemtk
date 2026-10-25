package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/repository"
)

// TestComputeHealthScoreTierDefaultsAreUnchanged 钉住「成功率三档参数化之后，
// 默认值下的扣分与原来写死的 50 / 80 / 95 逐格相同」。
//
// 这批改动的全部风险都在这里：如果某档的默认值或判定边界（< 还是 <=）动了，
// 存量账号的健康分会无声地整体偏移，而扣分只影响 weight 与 risk_level，
// 没有告警、不会有人发现。表里每个值都是边界的前后左右四格。
func TestComputeHealthScoreTierDefaultsAreUnchanged(t *testing.T) {
	_, db := newSSERuntimeParamFixture(t)

	// 只让成功率维度起作用：配额用率为 0（低于最低档 0.7）、错误数为 0、登录态正常。
	// 成功率单位是百分数（与 SuccessRate 字段同口径），不是 0~1 比例。
	scoreOf := func(successRate float64) int {
		return computeHealthScore(WeComLoginOnline, 0, successRate, 0)
	}

	// 默认 50 / 80 / 95，判定用 <：
	//   <50 扣 30、<80 扣 15、<95 扣 5、>=95 扣 0
	cases := []struct {
		rate float64
		want int
		why  string
	}{
		{0, 70, "0 落最严档，扣 30"},
		{49.99, 70, "49.99 仍落最严档"},
		{50, 85, "恰好 50 不扣最严档（判定是 <），落 -15 档"},
		{79.99, 85, "79.99 落 -15 档"},
		{80, 95, "恰好 80 落 -5 档"},
		{94.99, 95, "94.99 落 -5 档"},
		{95, 100, "恰好 95 不扣分（刚好达标）"},
		{100, 100, "满分不扣分"},
	}
	for _, c := range cases {
		if got := scoreOf(c.rate); got != c.want {
			t.Errorf("成功率 %v：want %d got %d（%s）", c.rate, c.want, got, c.why)
		}
	}
	_ = db
}

// TestComputeHealthScoreQuotaTierDefaultsAreUnchanged 钉住配额三档参数化之后
// 默认值下的扣分与原来写死的 0.7 / 0.9 / 0.95 逐格相同。判定用 >（用满了才扣），
// 与成功率的 < 是不同方向，边界值必须各自单独覆盖。
func TestComputeHealthScoreQuotaTierDefaultsAreUnchanged(t *testing.T) {
	// 成功率满分、错误数为 0、登录态正常，只让配额维度起作用。
	scoreOf := func(quotaRate float64) int {
		return computeHealthScore(WeComLoginOnline, quotaRate, 100, 0)
	}
	cases := []struct {
		rate float64
		want int
		why  string
	}{
		{0, 100, "未用配额不扣分"},
		{0.7, 100, "恰好 0.7 不扣（判定是 >，用满了才开始扣）"},
		{0.71, 95, "刚过 0.7 落 -5 档"},
		{0.9, 95, "恰好 0.9 仍落 -5 档"},
		{0.91, 85, "刚过 0.9 落 -15 档"},
		{0.95, 85, "恰好 0.95 仍落 -15 档"},
		{0.96, 75, "刚过 0.95 落 -25 档"},
		{1.0, 75, "配额全满落 -25 档"},
	}
	for _, c := range cases {
		if got := scoreOf(c.rate); got != c.want {
			t.Errorf("配额用率 %v：want %d got %d（%s）", c.rate, c.want, got, c.why)
		}
	}
}

// TestRuntimeWeComSuccessRateTiersClampsOutOfOrder 钉住「运维乱序填档位不会把
// 扣分方向搞反」这个设计约束。
//
// 判定是顺序 if/else if 命中，若直接采用配置值，配成 critical=0.9 / warn=0.5
// 就会出现「成功率 0.6 落第一档扣 30 分，而成功率 0.3 反而落到最后一档扣 5 分」
// —— 比不分档更糟。runtimeWeComSuccessRateTiers 用 clamp 强制 critical <
// degrade < warn，这里逐格验证 clamp 真的挡住了三种填法。
func TestRuntimeWeComSuccessRateTiersClampsOutOfOrder(t *testing.T) {
	_, db := newSSERuntimeParamFixture(t)
	ctx := context.Background()

	set := func(critical, degrade, warn string) {
		t.Helper()
		repo := repository.NewConfigParamRepository(db)
		for _, kv := range [][2]string{
			{"success_rate_critical", critical},
			{"success_rate_degrade", degrade},
			{"success_rate_warn", warn},
		} {
			if err := repo.UpdateValue(ctx, "wecom", kv[0], kv[1], 1); err != nil {
				t.Fatalf("写 %s=%s 失败：%v", kv[0], kv[1], err)
			}
		}
		// 冷缓存：参数服务按 group.key 缓存 60s，否则读到上一格的旧值。
		SetGlobalForTest(NewConfigParamService())
	}

	cases := []struct {
		name                                string
		critical, degrade, warn             string
		wantCritical, wantDegrade, wantWarn float64
		why                                 string
	}{
		{"默认顺序（百分数）", "50", "80", "95", 50, 80, 95, "顺序正确时原样采用"},
		{"degrade 填得比 warn 宽松", "50", "99", "80", 50, 80, 80, "degrade 被夹到 warn"},
		{"critical 填得比 degrade 宽松", "90", "80", "95", 80, 80, 95, "critical 被夹到 degrade"},
		{"三档全部填成同一个值", "70", "70", "70", 70, 70, 70, "全等是合法的退化情形，不改也不行"},
		{"degrade 超过 100", "50", "140", "95", 50, 95, 95, "先夹进 [0,100] 再递减"},
		{"负值夹到 0", "50", "-5", "95", 0, 0, 95, "负值先夹到 0，再被 degrade 的 0 反向夹住"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			set(c.critical, c.degrade, c.warn)
			gotCritical, gotDegrade, gotWarn := runtimeWeComSuccessRateTiers(ctx)
			if gotCritical != c.wantCritical || gotDegrade != c.wantDegrade || gotWarn != c.wantWarn {
				t.Fatalf("%s：want critical=%v degrade=%v warn=%v，got critical=%v degrade=%v warn=%v",
					c.why, c.wantCritical, c.wantDegrade, c.wantWarn, gotCritical, gotDegrade, gotWarn)
			}
			// 无论怎么填，递减关系必须成立——这是这套 clamp 存在的全部意义。
			if !(gotCritical <= gotDegrade && gotDegrade <= gotWarn) {
				t.Fatalf("递减关系被破坏：critical=%v degrade=%v warn=%v", gotCritical, gotDegrade, gotWarn)
			}
		})
	}
}

// TestRuntimeWeComQuotaTiersClampsOutOfOrder 同上，配额侧（判定方向相反，
// 需要 minRate <= degrade <= criticalRate）。
func TestRuntimeWeComQuotaTiersClampsOutOfOrder(t *testing.T) {
	_, db := newSSERuntimeParamFixture(t)
	ctx := context.Background()

	set := func(minRate, degrade, critical string) {
		t.Helper()
		repo := repository.NewConfigParamRepository(db)
		for _, kv := range [][2]string{
			{"quota_min_rate", minRate},
			{"quota_degrade", degrade},
			{"quota_critical_rate", critical},
		} {
			if err := repo.UpdateValue(ctx, "wecom", kv[0], kv[1], 1); err != nil {
				t.Fatalf("写 %s=%s 失败：%v", kv[0], kv[1], err)
			}
		}
		SetGlobalForTest(NewConfigParamService())
	}

	cases := []struct {
		name                               string
		minRate, degrade, critical         string
		wantMin, wantDegrade, wantCritical float64
		why                                string
	}{
		{"默认顺序", "0.7", "0.9", "0.95", 0.7, 0.9, 0.95, "顺序正确时原样采用"},
		{"minRate 填得比 degrade 宽松", "0.95", "0.9", "0.99", 0.95, 0.95, 0.99, "minRate 与 degrade 都抬高到 0.95 保持不交叉"},
		{"critical 填得比 degrade 宽松", "0.7", "0.99", "0.8", 0.7, 0.99, 0.99, "degrade 先保住 0.99，critical 被抬到 0.99 保持递增"},
		{"三档全部填成同一个值", "0.8", "0.8", "0.8", 0.8, 0.8, 0.8, "全等是合法的退化情形"},
		{"degrade 低于 0", "0.7", "-0.5", "0.95", 0.7, 0.7, 0.95, "负值先夹进 [0,1]，再被抬回 minRate 保持不交叉"},
		{"minRate 超过 1", "1.4", "0.9", "0.95", 1, 1, 1, "全部夹到 1，是合法的最严配置"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			set(c.minRate, c.degrade, c.critical)
			gotMin, gotDegrade, gotCritical := runtimeWeComQuotaTiers(ctx)
			if gotMin != c.wantMin || gotDegrade != c.wantDegrade || gotCritical != c.wantCritical {
				t.Fatalf("%s：want min=%v degrade=%v critical=%v，got min=%v degrade=%v critical=%v",
					c.why, c.wantMin, c.wantDegrade, c.wantCritical, gotMin, gotDegrade, gotCritical)
			}
			if !(gotMin <= gotDegrade && gotDegrade <= gotCritical) {
				t.Fatalf("递增关系被破坏：min=%v degrade=%v critical=%v", gotMin, gotDegrade, gotCritical)
			}
		})
	}
}

// TestWeComHealthTierSeedsMatchFallbacks 反向钉死：种子的默认值必须与代码兜底常量
// 相同。两者不一致意味着「页面显示的默认」与「参数中心不可用时的实际行为」会分叉，
// 而这类分叉只有等参数中心真的读失败那天才会暴露。
func TestWeComHealthTierSeedsMatchFallbacks(t *testing.T) {
	want := map[string]string{
		"wecom.quota_min_rate":        "0.7",
		"wecom.quota_degrade":         "0.9",
		"wecom.quota_critical_rate":   "0.95",
		"wecom.success_rate_critical": "50",
		"wecom.success_rate_degrade":  "80",
		"wecom.success_rate_warn":     "95",
	}
	got := map[string]string{}
	for _, d := range DefaultParamDefs() {
		coord := d.Group + "." + d.Key
		if _, ok := want[coord]; ok {
			got[coord] = d.DefaultValue
		}
	}
	for coord, wantVal := range want {
		gotVal, ok := got[coord]
		if !ok {
			t.Errorf("%s 不在 DefaultParamDefs() 里", coord)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("%s 的种子默认值是 %s，与代码兜底 %s 不一致", coord, gotVal, wantVal)
		}
	}
}

// TestRetiredPersonaAndErrorRateParamsAreGone 钉死三条下架的参数不会被人重新加回来。
//
// 下架的三条各有具体理由：persona 两条的实现已随 9a5f716c 删除；error_rate_degrade
// 「单一阈值表达不了递减分档」的正解是拆三条而不是放弃，而且它叫「错误率」而代码
// 判定用的是「成功率」，互补数拿一个当另一个用会得出方向相反的结论。
// 这条测试的价值在于：将来有人看到页面上少了一个旋钮、代码里又没有，会想当然地
// 把它补回去——而补回去的只会是一个改了永远不生效的僵尸参数。
func TestRetiredPersonaAndErrorRateParamsAreGone(t *testing.T) {
	retired := []string{
		"confidence.persona_default_threshold",
		"confidence.persona_max_retry",
		"wecom.error_rate_degrade",
	}
	present := map[string]bool{}
	for _, d := range DefaultParamDefs() {
		present[d.Group+"."+d.Key] = true
	}
	for _, coord := range retired {
		if present[coord] {
			t.Errorf("%s 已下架，不该再出现在 DefaultParamDefs() 里", coord)
		}
	}
}
