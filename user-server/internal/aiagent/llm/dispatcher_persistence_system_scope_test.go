package llm

import (
	"context"
	"encoding/json"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// 回归钉死 TENANT_MODE=on 时的启动期 fail-closed 缺陷（生产日志实证）：
//
//	[LLM] 从数据库加载 provider 失败：[tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（query）
//	[LLM] LoadRoutesFromDB 查询失败: [tenant] 表 llm_routing_rules 需要租户上下文但 ctx 未绑定（query）
//
// 启动期/后台加载没有 HTTP 请求 ctx 可绑，必须走 tenant.WithSystemScope 这条唯一
// 豁免通道（与 migration/备份/健康检查同类的平台级装配操作）；同时隔离本身不放宽：
// 普通无租户 ctx 的查询仍然 fail-closed（下面第一条断言）。
func TestLoadProvidersFromDB_OnModeWithoutTenantCtx(t *testing.T) {
	database := testutil.NewTestDB(t, &model.LLMProvider{}, &model.LLMRoutingRule{})
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	// seed：与后台 admin API 同语义，带租户 ctx 写入
	if err := database.WithContext(tenant.WithTenantID(context.Background(), 1)).
		Create(&model.LLMProvider{
			Name:    "cloud-bootstrap",
			BaseURL: "https://api.example.com/v1",
			Model:   "test-model",
			APIType: "openai",
			Enabled: true,
		}).Error; err != nil {
		t.Fatalf("seed provider 失败: %v", err)
	}

	// 隔离闸仍在：无租户 ctx 的裸查询必须 fail-closed（本修复不得放宽 HTTP 请求路径）
	if err := database.WithContext(context.Background()).Find(&[]model.LLMProvider{}).Error; err == nil {
		t.Fatal("on 模式无租户 ctx 的裸查询必须 fail-closed（隔离不能被本修复放宽）")
	}

	setAuditDB(database)
	defer setAuditDB(nil)

	d := newDispatcherBase(nil)
	if err := d.LoadProvidersFromDB(); err != nil {
		t.Fatalf("启动期（无租户 ctx）加载 provider 必须在 system scope 下成功: %v", err)
	}

	var loaded bool
	for _, p := range d.GetAllProviders() {
		if p.Name == "cloud-bootstrap" {
			loaded = true
			break
		}
	}
	if !loaded {
		t.Fatal("LoadProvidersFromDB 应把 DB 中的 provider 装配进 dispatcher（否则只剩 config.yaml 种子）")
	}
}

func TestLoadRoutesFromDB_OnModeWithoutTenantCtx(t *testing.T) {
	database := testutil.NewTestDB(t, &model.LLMProvider{}, &model.LLMRoutingRule{})
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	want := ScenarioRoute{Scenario: ScenarioLowCost, Provider: "cloud-bootstrap", Version: 7}
	buf, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal route: %v", err)
	}
	if err := database.WithContext(tenant.WithTenantID(context.Background(), 1)).
		Create(&model.LLMRoutingRule{
			Scenario:  string(ScenarioLowCost),
			RouteJSON: string(buf),
			Version:   want.Version,
		}).Error; err != nil {
		t.Fatalf("seed routing rule 失败: %v", err)
	}

	setAuditDB(database)
	defer setAuditDB(nil)

	// 无默认种子的干净 dispatcher：LoadRoutesFromDB 是唯一来源
	d := newDispatcherBase(nil)
	if err := d.LoadRoutesFromDB(); err != nil {
		t.Fatalf("启动期（无租户 ctx）加载路由必须在 system scope 下成功: %v", err)
	}
	got := d.GetRoute(ScenarioLowCost)
	if got == nil || got.Provider != "cloud-bootstrap" || got.Version != 7 {
		t.Fatalf("LoadRoutesFromDB 应覆盖加载 DB 路由，实际 %+v", got)
	}
}
