package middleware

import (
	"context"
	"encoding/json"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

// TestPathQuotasSeedDefaultMatchesCodeDefault 种子 DefaultValue 必须与
// middleware.DefaultPathQuota 语义等价（逐字节不可比——JSON 键序不定，比键集合与数值）。
func TestPathQuotasSeedDefaultMatchesCodeDefault(t *testing.T) {
	var seedDef string
	for _, d := range service.DefaultParamDefs() {
		if d.Group == "ratelimit" && d.Key == "path_quotas" {
			seedDef = d.DefaultValue
		}
	}
	if seedDef == "" {
		t.Fatal("种子 ratelimit.path_quotas 不存在")
	}
	var fromSeed map[string]pathQuotaEntry
	if err := json.Unmarshal([]byte(seedDef), &fromSeed); err != nil {
		t.Fatalf("种子默认值不是合法 JSON: %v", err)
	}
	if len(fromSeed) != len(DefaultPathQuota) {
		t.Errorf("种子默认条数 = %d, 代码默认 = %d", len(fromSeed), len(DefaultPathQuota))
	}
	for path, want := range DefaultPathQuota {
		got, ok := fromSeed[path]
		if !ok {
			t.Errorf("种子默认缺少路径 %s", path)
			continue
		}
		if got.RPS != want.RPS || got.BucketSize != want.BucketSize {
			t.Errorf("种子默认 %s = rps %v/bucket %d, 代码 = rps %v/bucket %d",
				path, got.RPS, got.BucketSize, want.RPS, want.BucketSize)
		}
	}
}

// TestResolvePathQuotas_NoParamFallsBack 参数未装配（GlobalConfigParam 是无 DB stub，
// 读不到值）时回落 DefaultPathQuota——行为与接线前逐字节一致。
func TestResolvePathQuotas_NoParamFallsBack(t *testing.T) {
	service.SetGlobalForTest(nil)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	got := resolvePathQuotas(context.Background())
	if len(got) != len(DefaultPathQuota) {
		t.Fatalf("无参数时条数 = %d, 期望回落默认 %d", len(got), len(DefaultPathQuota))
	}
	for path, want := range DefaultPathQuota {
		if got[path] != want {
			t.Errorf("无参数时 %s = %+v, 期望 %+v", path, got[path], want)
		}
	}
}

// TestResolvePathQuotas_ParamOverrides 参数库有值时用参数值（证明读取点用同一个
// group.key，不会因 key 拼错而静默回落）。
func TestResolvePathQuotas_ParamOverrides(t *testing.T) {
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if database == nil {
		t.Skip("no test DB")
	}
	if err := service.SeedConfigParams(context.Background()); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := service.NewConfigParamService()
	service.SetGlobalForTest(svc)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })

	ctx := context.Background()
	custom := `{"/api/custom":{"rps":9,"bucket_size":99}}`
	if err := svc.UpdateValue(ctx, "ratelimit", "path_quotas", custom, 1); err != nil {
		t.Fatalf("改 ratelimit.path_quotas 失败：%v", err)
	}
	got := resolvePathQuotas(ctx)
	if len(got) != 1 {
		t.Fatalf("覆写后条数 = %d, 期望 1", len(got))
	}
	cfg, ok := got["/api/custom"]
	if !ok || cfg.RPS != 9 || cfg.BucketSize != 99 || cfg.Path != "/api/custom" {
		t.Errorf("覆写后 /api/custom = %+v, 期望 rps 9/bucket 99/Path 回填", cfg)
	}

	// 坏 JSON：回落默认表而不是空表（空表=限流全关，比坏 JSON 更危险）。
	if err := svc.UpdateValue(ctx, "ratelimit", "path_quotas", `{broken`, 1); err == nil {
		t.Error("坏 JSON 应被写侧 validateValue 拦下（json.Valid）")
	}
}

// TestResolvePathQuotas_DropsInvalidEntries 单条非法（路径不以 / 开头、rps<=0、
// bucket<=0）丢弃该条，其余照常；全坏则回落默认表。
func TestResolvePathQuotas_DropsInvalidEntries(t *testing.T) {
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if database == nil {
		t.Skip("no test DB")
	}
	if err := service.SeedConfigParams(context.Background()); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := service.NewConfigParamService()
	service.SetGlobalForTest(svc)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	ctx := context.Background()

	// 一好一坏：写侧只验 JSON 语法，结构校验在读取侧。
	mixed := `{"/api/good":{"rps":2,"bucket_size":4},"api-no-slash":{"rps":2,"bucket_size":4},"/api/zero":{"rps":0,"bucket_size":4}}`
	if err := svc.UpdateValue(ctx, "ratelimit", "path_quotas", mixed, 1); err != nil {
		t.Fatalf("写 mixed 失败：%v", err)
	}
	got := resolvePathQuotas(ctx)
	if len(got) != 1 {
		t.Fatalf("混合合法/非法条目后条数 = %d, 期望只留 /api/good", len(got))
	}
	if _, ok := got["/api/good"]; !ok {
		t.Error("合法条目 /api/good 应保留")
	}

	// 全坏：回落默认表。
	allBad := `{"no-slash":{"rps":1,"bucket_size":1}}`
	if err := svc.UpdateValue(ctx, "ratelimit", "path_quotas", allBad, 1); err != nil {
		t.Fatalf("写 allBad 失败：%v", err)
	}
	got = resolvePathQuotas(ctx)
	if len(got) != len(DefaultPathQuota) {
		t.Errorf("全坏条目时应回落默认表，实际条数 = %d", len(got))
	}
}
