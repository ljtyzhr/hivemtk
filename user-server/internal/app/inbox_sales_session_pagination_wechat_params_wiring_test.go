package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/aiagent/mcp"
	contentsvc "hivemtk-user/internal/content/service"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/pagination"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
	tracelearn "hivemtk-user/internal/service/trace_learning"
)

// wantInboxSalesSessionPaginationWired 是本批接线的 10 条。写死而不是从
// DefaultParamDefs 反推：反推的话，键名拼错也会被算进本组，漏接的那条就永远没人发现。
var wantInboxSalesSessionPaginationWired = []string{
	"inbox_sales.tg_lead_opportunity_threshold",
	"inbox_sales.unified_miner_lead_threshold",
	"lead.llm_refine_enabled",
	"lead.industry_profile",
	"inbox_sales.preview_sample_limit",
	"inbox_sales.geo_lead_preview_max_len",
	"session.active_ttl",
	"session.idle_ttl",
	"session.max_delay_seconds",
	"pagination.cursor_page_size",
}

func resetInboxSalesSessionPaginationProviders(t *testing.T) {
	t.Helper()
	// lead 那两条的 setter 对 nil 是「不注入」，所以清理只能装回默认值本身，
	// 否则上一个测试装的闭包会带着 GlobalConfigParam 活到下一个测试里。
	t.Cleanup(func() {
		service.SetTgLeadOpportunityThresholdProvider(nil)
		service.SetUnifiedMinerOpportunityThresholdProvider(nil)
		service.SetPreviewSampleLimitProvider(nil)
		service.SetLeadLLMRefineProviders(
			func() bool { return service.DefaultLeadLLMRefineEnabled },
			func() string { return service.DefaultLeadIndustryProfile },
		)
		tracelearn.SetInsightMaxLenProvider(nil)
		repository.SetSessionActiveTTLProvider(nil)
		mcp.SetSessionIdleTTLProvider(nil)
		contentsvc.SetMaxFlowDelaySecondsProvider(nil)
		pagination.SetCursorPageSizeProvider(nil)
	})
}

func newInboxSalesSessionPaginationParamFixture(t *testing.T) *service.ConfigParamService {
	t.Helper()
	resetInboxSalesSessionPaginationProviders(t)
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if database == nil {
		t.Skip("no DB")
	}
	if err := service.SeedConfigParams(context.Background()); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := service.NewConfigParamService()
	service.SetGlobalForTest(svc)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	return svc
}

// wiredCase 一张表一行：接线键名、读口当前值、期望值，以及改动前的值（只进失败文案）。
// 两张表（有库翻转／无库回落）共用同一个形状，才可以让 assertCoversWiredKeys 检查覆盖率。
type wiredCase struct {
	name      string
	got, want any
	was       any
}

// assertCoversWiredKeys 断言这张表逐条覆盖了 wantInboxSalesSessionPaginationWired。
// 表是手写的：漏一条不会报错，只会安静地少测一个键——而「接线了却没人读」正是这批
// 参数出过的事故（lead 两条被塞进另一条参数的读取副作用里，用例照样全绿）。
func assertCoversWiredKeys(t *testing.T, label string, cases []wiredCase) {
	t.Helper()
	want := map[string]bool{}
	for _, k := range wantInboxSalesSessionPaginationWired {
		i := strings.Index(k, ".")
		want[k[i+1:]] = true
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if !want[c.name] {
			t.Errorf("%s 里的 %s 不在接线名单中：名单与用例不同步", label, c.name)
		}
		seen[c.name] = true
	}
	covered := 0
	for k := range want {
		if seen[k] {
			covered++
		}
	}
	for k := range want {
		if !seen[k] {
			t.Errorf("%s 少 %s 这一腿：接线 %d 条，本表只覆盖 %d 条", label, k, len(want), covered)
		}
	}
}

func TestWireInboxSalesSessionPaginationConfigParamsKeys(t *testing.T) {
	resetInboxSalesSessionPaginationProviders(t)
	got := WireInboxSalesSessionPaginationConfigParams()
	if len(got) != len(wantInboxSalesSessionPaginationWired) {
		t.Fatalf("接线条数 = %d，期望 %d\ngot  = %v\nwant = %v", len(got), len(wantInboxSalesSessionPaginationWired), got, wantInboxSalesSessionPaginationWired)
	}
	seen := map[string]bool{}
	for i, k := range got {
		if k != wantInboxSalesSessionPaginationWired[i] {
			t.Errorf("第 %d 条接线 = %q，期望 %q", i, k, wantInboxSalesSessionPaginationWired[i])
		}
		if seen[k] {
			t.Errorf("键 %q 被接了两次", k)
		}
		seen[k] = true
	}
}

func TestWiredInboxSalesSessionPaginationKeysExistInSeed(t *testing.T) {
	index := map[string]bool{}
	for _, d := range service.DefaultParamDefs() {
		index[d.Group+"."+d.Key] = true
	}
	for _, k := range wantInboxSalesSessionPaginationWired {
		if !index[k] {
			t.Errorf("%q 在 DefaultParamDefs() 里不存在", k)
		}
	}
}

func TestWireInboxSalesSessionPaginationConfigParamsReachesReaders(t *testing.T) {
	svc := newInboxSalesSessionPaginationParamFixture(t)
	ctx := context.Background()

	beforeTg := service.ProbeTgLeadOpportunityThreshold()
	beforeUnified := service.ProbeUnifiedMinerOpportunityThreshold()
	beforeRefine := service.ProbeLeadLLMRefineEnabled()
	beforeProfile := service.ProbeLeadIndustryProfile()
	beforePreview := service.ProbePreviewSampleLimit()
	beforeInsight := tracelearn.InsightMaxLen()
	beforeSession := repository.SessionActiveTTL()
	beforeIdle := mcp.SessionIdleTTL()
	beforeDelay := contentsvc.ProbeMaxFlowDelaySeconds()
	beforeCursor := pagination.CursorPageSize()

	WireInboxSalesSessionPaginationConfigParams()

	for _, tc := range []struct{ group, key, val string }{
		{"inbox_sales", "tg_lead_opportunity_threshold", "65"},
		{"inbox_sales", "unified_miner_lead_threshold", "66"},
		{"lead", "llm_refine_enabled", "false"},
		{"lead", "industry_profile", "SaaS 订阅售前咨询：客户关心试用、席位数与年付折扣"},
		{"inbox_sales", "preview_sample_limit", "7"},
		{"inbox_sales", "geo_lead_preview_max_len", "333"},
		{"session", "active_ttl", "172800"},
		{"session", "idle_ttl", "600"},
		{"session", "max_delay_seconds", "45"},
		{"pagination", "cursor_page_size", "555"},
	} {
		if err := svc.UpdateValue(ctx, tc.group, tc.key, tc.val, 1); err != nil {
			t.Fatalf("改 %s.%s 失败：%v", tc.group, tc.key, err)
		}
	}

	cases := []wiredCase{
		{"tg_lead_opportunity_threshold", service.ProbeTgLeadOpportunityThreshold(), 65, beforeTg},
		{"unified_miner_lead_threshold", service.ProbeUnifiedMinerOpportunityThreshold(), 66, beforeUnified},
		// lead 两条单独验一次：它们曾经只在「读阈值」这个副作用里才被注入，
		// 换个读取顺序就会拿着包级默认值，所以这里必须断言它们**不依赖**别的参数被读过。
		{"llm_refine_enabled", service.ProbeLeadLLMRefineEnabled(), false, beforeRefine},
		{"industry_profile", service.ProbeLeadIndustryProfile(), "SaaS 订阅售前咨询：客户关心试用、席位数与年付折扣", beforeProfile},
		{"preview_sample_limit", service.ProbePreviewSampleLimit(), 7, beforePreview},
		{"geo_lead_preview_max_len", tracelearn.InsightMaxLen(), 333, beforeInsight},
		{"active_ttl", repository.SessionActiveTTL(), 48 * time.Hour, beforeSession},
		{"idle_ttl", mcp.SessionIdleTTL(), 10 * time.Minute, beforeIdle},
		// 种子类型是 duration（秒），折算后应当是 45 秒。
		{"max_delay_seconds", contentsvc.ProbeMaxFlowDelaySeconds(), 45, beforeDelay},
		{"cursor_page_size", pagination.CursorPageSize(), 555, beforeCursor},
	}
	assertCoversWiredKeys(t, "有库翻转表", cases)
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s 未被读走：got %v（原 %v）want %v", tc.name, tc.got, tc.was, tc.want)
		}
	}
}

func TestWireInboxSalesSessionPaginationConfigParamsWithoutDB(t *testing.T) {
	service.SetGlobalForTest(nil)
	resetInboxSalesSessionPaginationProviders(t)

	if got := WireInboxSalesSessionPaginationConfigParams(); len(got) != len(wantInboxSalesSessionPaginationWired) {
		t.Fatalf("无库时接线条数 = %d，期望 %d", len(got), len(wantInboxSalesSessionPaginationWired))
	}
	cases := []wiredCase{
		{"tg_lead_opportunity_threshold", service.ProbeTgLeadOpportunityThreshold(), service.DefaultTgLeadOpportunityThreshold, nil},
		{"unified_miner_lead_threshold", service.ProbeUnifiedMinerOpportunityThreshold(), service.DefaultUnifiedMinerOpportunityThreshold, nil},
		{"llm_refine_enabled", service.ProbeLeadLLMRefineEnabled(), service.DefaultLeadLLMRefineEnabled, nil},
		{"industry_profile", service.ProbeLeadIndustryProfile(), service.DefaultLeadIndustryProfile, nil},
		{"preview_sample_limit", service.ProbePreviewSampleLimit(), service.DefaultPreviewSampleLimit, nil},
		{"geo_lead_preview_max_len", tracelearn.InsightMaxLen(), tracelearn.DefaultInsightMaxLen, nil},
		{"active_ttl", repository.SessionActiveTTL(), repository.DefaultSessionActiveTTL, nil},
		{"idle_ttl", mcp.SessionIdleTTL(), mcp.DefaultSessionIdleTTL, nil},
		{"max_delay_seconds", contentsvc.ProbeMaxFlowDelaySeconds(), contentsvc.DefaultMaxFlowDelaySeconds, nil},
		{"cursor_page_size", pagination.CursorPageSize(), pagination.DefaultCursorPageSize, nil},
	}
	assertCoversWiredKeys(t, "无库回落表", cases)
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("无库时 %s = %v，期望兜底 %v", tc.name, tc.got, tc.want)
		}
	}
}
