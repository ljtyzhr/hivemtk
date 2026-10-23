// selector_health_test.go B1-a：选择器失效检测的**判定面**用例。
//
// 判据全部落在「会不会把平台改版误判成没客户」这条主线上：
//
//	· 样本不足不许判 broken（否则新装扩展第一次打开页面就红，运营很快学会无视红色）；
//	· 一次都没命中才是 broken（这是「选择器彻底失效」的形状）；
//	· 阈值边界两侧必须分开（等于阈值算 ok：阈值写 60 的语义是「低于 60 才降级」）；
//	· 上报形状非法必须 400（静默吞掉＝扩展侧写错了永远没人知道）；
//	· 增量上限是**夹**不是丢（丢掉分母会让命中率偏高，正好掩盖问题）。
package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// fakeSelectorHealthRepo 内存仓储：本组用例只判「校验 + 判定 + 夹取」，
// 不判 SQL 落地（那部分在 repository 的真库用例里）。
type fakeSelectorHealthRepo struct {
	rows []model.BridgeSelectorHealth
	err  error
}

func (f *fakeSelectorHealthRepo) Upsert(ctx context.Context, deltas []repository.SelectorHealthDelta) error {
	if f.err != nil {
		return f.err
	}
	for _, d := range deltas {
		found := false
		for i := range f.rows {
			if f.rows[i].Channel == d.Channel && f.rows[i].Field == d.Field {
				f.rows[i].Attempts += d.Attempts
				f.rows[i].Hits += d.Hits
				f.rows[i].LastReportAt = d.ReportedAt
				found = true
				break
			}
		}
		if found {
			continue
		}
		f.rows = append(f.rows, model.BridgeSelectorHealth{
			Channel: d.Channel, Field: d.Field,
			Attempts: d.Attempts, Hits: d.Hits,
			FirstSeenAt: d.ReportedAt, LastReportAt: d.ReportedAt,
		})
	}
	return nil
}

func (f *fakeSelectorHealthRepo) List(ctx context.Context) ([]model.BridgeSelectorHealth, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

func newSelectorHealthSvc() (*SelectorHealthService, *fakeSelectorHealthRepo) {
	repo := &fakeSelectorHealthRepo{}
	svc := NewSelectorHealthService(repo)
	svc.now = func() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) }
	return svc, repo
}

func TestClassifySelectorHealth_WarmingThenBrokenThenDegraded(t *testing.T) {
	tests := []struct {
		name     string
		attempts uint64
		hits     uint64
		below    int
		want     string
	}{
		{"零样本 ⇒ warming（不是 broken）", 0, 0, 60, SelectorHealthStatusWarming},
		{"样本不足 ⇒ warming", uint64(SelectorHealthMinAttempts - 1), 0, 60, SelectorHealthStatusWarming},
		{"样本够了且零命中 ⇒ broken", uint64(SelectorHealthMinAttempts), 0, 60, SelectorHealthStatusBroken},
		{"刚好 59% ⇒ degraded", 100, 59, 60, SelectorHealthStatusDegraded},
		{"刚好 60% ⇒ ok（阈值是「低于」）", 100, 60, 60, SelectorHealthStatusOK},
		{"100% ⇒ ok", 100, 100, 60, SelectorHealthStatusOK},
		{"阈值非法 0 ⇒ 用兜底 60，59% 仍 degraded", 100, 59, 0, SelectorHealthStatusDegraded},
		{"阈值非法 101 ⇒ 用兜底 60，99% 仍 ok", 100, 99, 101, SelectorHealthStatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifySelectorHealth(tt.attempts, tt.hits, tt.below); got != tt.want {
				t.Fatalf("ClassifySelectorHealth(%d,%d,%d) = %s, want %s",
					tt.attempts, tt.hits, tt.below, got, tt.want)
			}
		})
	}
}

func TestSelectorHealthHitRate_ZeroAttemptsIsZeroNotNaN(t *testing.T) {
	if got := selectorHitRate(0, 0); got != 0 {
		t.Fatalf("0 尝试的命中率应为 0，实际 %v", got)
	}
	if got := selectorHitRate(3, 1); got != 33.3 {
		t.Fatalf("1/3 应保留一位小数 33.3，实际 %v", got)
	}
}

func TestSelectorHealthIngest_RejectsMalformedReports(t *testing.T) {
	tests := []struct {
		name string
		rep  SelectorHealthReport
	}{
		{"空 channel", SelectorHealthReport{Channel: "", Items: []SelectorHealthObservation{{Field: "messageItem", Attempts: 1}}}},
		{"channel 形状非法（大写）", SelectorHealthReport{Channel: "DouYin", Items: []SelectorHealthObservation{{Field: "messageItem", Attempts: 1}}}},
		{"channel 含空格", SelectorHealthReport{Channel: "dou yin", Items: []SelectorHealthObservation{{Field: "messageItem", Attempts: 1}}}},
		{"items 为空", SelectorHealthReport{Channel: "douyin"}},
		{"field 不在白名单", SelectorHealthReport{Channel: "douyin", Items: []SelectorHealthObservation{{Field: "whatever", Attempts: 1}}}},
		{"同一次上报里 field 重复", SelectorHealthReport{Channel: "douyin", Items: []SelectorHealthObservation{
			{Field: "messageItem", Attempts: 1}, {Field: "messageItem", Attempts: 2},
		}}},
		{"hits > attempts（计数口径不一致）", SelectorHealthReport{Channel: "douyin", Items: []SelectorHealthObservation{
			{Field: "messageItem", Attempts: 1, Hits: 2},
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newSelectorHealthSvc()
			_, err := svc.Ingest(context.Background(), tt.rep)
			if !errors.Is(err, ErrSelectorHealthInvalidReport) {
				t.Fatalf("期望 ErrSelectorHealthInvalidReport，实际 %v", err)
			}
			if len(repo.rows) != 0 {
				t.Fatalf("被拒的上报不许落库，实际写入 %d 行", len(repo.rows))
			}
		})
	}
}

func TestSelectorHealthIngest_TooManyItems(t *testing.T) {
	svc, _ := newSelectorHealthSvc()
	rep := SelectorHealthReport{Channel: "douyin"}
	for i := 0; i <= SelectorHealthMaxItemsPerReport; i++ {
		rep.Items = append(rep.Items, SelectorHealthObservation{Field: fmt.Sprintf("f%d", i), Attempts: 1})
	}
	if _, err := svc.Ingest(context.Background(), rep); !errors.Is(err, ErrSelectorHealthInvalidReport) {
		t.Fatalf("超过上限应报 ErrSelectorHealthInvalidReport，实际 %v", err)
	}
}

func TestSelectorHealthIngest_ClampsHugeDeltaKeepsRatioSane(t *testing.T) {
	svc, repo := newSelectorHealthSvc()
	_, err := svc.Ingest(context.Background(), SelectorHealthReport{
		Channel: "douyin",
		Items: []SelectorHealthObservation{
			{Field: "messageItem", Attempts: SelectorHealthMaxDeltaPerReport * 3, Hits: SelectorHealthMaxDeltaPerReport * 3},
		},
	})
	if err != nil {
		t.Fatalf("合法上报不应报错：%v", err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("应有 1 行，实际 %d", len(repo.rows))
	}
	if repo.rows[0].Attempts != uint64(SelectorHealthMaxDeltaPerReport) {
		t.Fatalf("增量应被夹到 %d，实际 %d", SelectorHealthMaxDeltaPerReport, repo.rows[0].Attempts)
	}
	// 夹取必须**等比**：分子分母都被夹到同一个上限，命中率仍是 100%。
	if got := selectorHitRate(repo.rows[0].Attempts, repo.rows[0].Hits); got != 100 {
		t.Fatalf("夹取后命中率应仍为 100，实际 %v", got)
	}
}

func TestSelectorHealthList_StatusesAndFreshness(t *testing.T) {
	svc, repo := newSelectorHealthSvc()
	now := svc.now()
	repo.rows = []model.BridgeSelectorHealth{
		// 平台改版形状：试了 500 次一次没命中
		{Channel: "douyin", Field: "messageItem", Attempts: 500, Hits: 0, LastReportAt: now.Add(-5 * time.Minute)},
		// 部分失效
		{Channel: "douyin", Field: "conversationList", Attempts: 100, Hits: 20, LastReportAt: now},
		// 健康
		{Channel: "xhs", Field: "messageItem", Attempts: 100, Hits: 98, LastReportAt: now},
		// 刚接上、样本不足
		{Channel: "xhs", Field: "input", Attempts: 2, Hits: 0, LastReportAt: now},
	}
	views, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List 报错：%v", err)
	}
	if len(views) != 4 {
		t.Fatalf("应有 4 条读数，实际 %d", len(views))
	}
	byKey := map[string]SelectorHealthView{}
	for _, v := range views {
		byKey[v.Channel+"/"+v.Field] = v
	}
	if got := byKey["douyin/messageItem"].Status; got != SelectorHealthStatusBroken {
		t.Fatalf("500 次零命中应判 broken，实际 %s", got)
	}
	if got := byKey["douyin/conversationList"].Status; got != SelectorHealthStatusDegraded {
		t.Fatalf("20%% 应判 degraded，实际 %s", got)
	}
	if got := byKey["xhs/messageItem"].Status; got != SelectorHealthStatusOK {
		t.Fatalf("98%% 应判 ok，实际 %s", got)
	}
	if got := byKey["xhs/input"].Status; got != SelectorHealthStatusWarming {
		t.Fatalf("2 次样本应判 warming，实际 %s", got)
	}
	// 新鲜度：5 分钟前报的那条必须能看出「不是现在的读数」
	if got := byKey["douyin/messageItem"].StaleSeconds; got != 300 {
		t.Fatalf("陈旧读数应给出 300 秒，实际 %d", got)
	}
	if byKey["douyin/messageItem"].LastReportAt == "" {
		t.Fatal("LastReportAt 应为 RFC3339 字符串")
	}
}

func TestSelectorHealthIngest_ReturnsOnlyReportedChannel(t *testing.T) {
	svc, repo := newSelectorHealthSvc()
	repo.rows = []model.BridgeSelectorHealth{
		{Channel: "xhs", Field: "messageItem", Attempts: 100, Hits: 100},
	}
	views, err := svc.Ingest(context.Background(), SelectorHealthReport{
		Channel: "douyin",
		Items:   []SelectorHealthObservation{{Field: "messageItem", Attempts: 30, Hits: 0}},
	})
	if err != nil {
		t.Fatalf("Ingest 报错：%v", err)
	}
	if len(views) != 1 || views[0].Channel != "douyin" {
		t.Fatalf("只应返回本次上报渠道的读数，实际 %+v", views)
	}
	if views[0].Status != SelectorHealthStatusBroken {
		t.Fatalf("30 次零命中应判 broken，实际 %s", views[0].Status)
	}
}

func TestSelectorHealth_NilRepoIsExplicitErrorNotPanic(t *testing.T) {
	svc := NewSelectorHealthService(nil)
	if _, err := svc.Ingest(context.Background(), SelectorHealthReport{
		Channel: "douyin", Items: []SelectorHealthObservation{{Field: "messageItem", Attempts: 1}},
	}); !errors.Is(err, ErrSelectorHealthRepoNotReady) {
		t.Fatalf("仓储未注入应报 ErrSelectorHealthRepoNotReady，实际 %v", err)
	}
	if _, err := svc.List(context.Background()); !errors.Is(err, ErrSelectorHealthRepoNotReady) {
		t.Fatalf("仓储未注入应报 ErrSelectorHealthRepoNotReady，实际 %v", err)
	}
}

func TestSelectorHealthThreshold_FallsBackWhenParamMissing(t *testing.T) {
	svc, _ := newSelectorHealthSvc()
	// 参数中心未装配（用例环境无库）：必须回落代码兜底，而不是 0
	// （0 会让所有行恒 degraded —— 恒红的门等于没有门）。
	if got := svc.Threshold(context.Background()); got != SelectorHealthDegradedBelow {
		t.Fatalf("阈值应回落 %d，实际 %d", SelectorHealthDegradedBelow, got)
	}
}
