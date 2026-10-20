package service

import (
	"context"
	"encoding/json"
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
	// Restart 判定按键分口径：上面六条是装配期读一次（Restart=true）；
	// path_quotas 是每请求热读（middleware/rate_quota.go 逐请求解析 JSON），标 Restart=false 才对。
	hotRead := map[string]bool{"path_quotas": true}
	index := map[string]string{}
	for _, d := range DefaultParamDefs() {
		if d.Group == "ratelimit" {
			index[d.Key] = d.DefaultValue
			if hotRead[d.Key] {
				if d.Restart {
					t.Errorf("ratelimit.%s 是每请求热读键，不应标 Restart=true", d.Key)
				}
				continue
			}
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
	// path_quotas 的默认值必须与 middleware.DefaultPathQuota 同源（JSON 序列化那张表）。
	// 逐字节断言放 middleware 包的用例里（那边能直接引用 DefaultPathQuota），
	// 这里只锁条数：六条装配期键 + 一条热读 JSON。
	if len(index) != len(want)+1 {
		t.Errorf("ratelimit 组条数 = %d, 期望 %d（装配期 %d 条 + path_quotas）", len(index), len(want)+1, len(want))
	}
	if got, ok := index["path_quotas"]; !ok {
		t.Error("ratelimit.path_quotas 在 DefaultParamDefs() 里不存在")
	} else if !json.Valid([]byte(got)) {
		t.Errorf("ratelimit.path_quotas 默认值不是合法 JSON: %q", got)
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
