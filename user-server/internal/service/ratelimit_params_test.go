package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// TestRateLimitSeedDefaultsMatchProduction 锁住「限流 6 条的种子默认值 = 生产值」这条
// 行为零变更不变量。
//
// 全局两条（global_rps/global_bucket）的生产值来自 router.go 装配时传入的 1000/20000，
// 不是 ratelimit.go DefaultRateLimitConfig 的 10/100（那只是无参调用时的兜底，生产不走）。
// 访客四条来自 chat_routes.go 此前的无参调用，回落 DefaultVisitorRateLimitConfig 20/40/100/200。
func TestRateLimitSeedDefaultsMatchProduction(t *testing.T) {
	want := map[string]string{
		"global_rps":                 "1000",
		"global_bucket":              "20000",
		"visitor_per_ip_rps":         "20",
		"visitor_per_ip_bucket":      "40",
		"visitor_per_channel_rps":    "100",
		"visitor_per_channel_bucket": "200",
	}
	index := map[string]string{}
	for _, d := range DefaultParamDefs() {
		if d.Group == "ratelimit" {
			index[d.Key] = d.DefaultValue
			if !d.Restart {
				t.Errorf("ratelimit.%s 应标 Restart=true：装配期读一次，不重启不生效", d.Key)
			}
		}
	}
	for k, w := range want {
		if got, ok := index[k]; !ok {
			t.Errorf("ratelimit.%s 在 DefaultParamDefs() 里不存在", k)
		} else if got != w {
			t.Errorf("ratelimit.%s 默认值 = %q, 期望生产值 %q", k, got, w)
		}
	}
	if len(index) != len(want) {
		t.Errorf("ratelimit 组条数 = %d, 期望 %d", len(index), len(want))
	}
}

// TestRateLimitReadersPickUpParams 参数库里改了值，GlobalConfigParam 必须读到（证明
// 读取点用的是同一个 group.key，不会因 key 拼错而静默回落）。
func TestRateLimitReadersPickUpParams(t *testing.T) {
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if database == nil {
		t.Skip("no test DB")
	}
	if err := SeedConfigParams(context.Background(), database); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := NewConfigParamService(database)
	SetGlobalForTest(svc)
	t.Cleanup(func() { SetGlobalForTest(nil) })

	ctx := context.Background()
	if err := svc.UpdateValue(ctx, "ratelimit", "global_rps", "500", 1); err != nil {
		t.Fatalf("改 ratelimit.global_rps 失败：%v", err)
	}
	if got := svc.GetInt(ctx, "ratelimit", "global_rps", 1000); got != 500 {
		t.Errorf("ratelimit.global_rps = %d, 期望 500（改库后应生效）", got)
	}
}
