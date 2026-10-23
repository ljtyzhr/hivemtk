package llm

import (
	"context"
	"encoding/json"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// 回归钉死 TENANT_MODE=on 时「后台改 LLM 路由 / provider 配置落库失败」的缺陷
// （生产日志实证，2026-10-22）：
//
//	[LLM] UpsertRouteToDB 查询 scenario=intent_recognize 失败: [tenant] 表 llm_routing_rules 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
//	[LLM] SetRouteWithAudit 路由落库失败 scenario=intent_recognize: [tenant] 表 llm_routing_rules 需要租户上下文但 ctx 未绑定（query）；...
//	{"code":400,"message":"provider 落库失败: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（query）；..."}
//
// 后果：接口返回成功（路由）或 400（provider），但**表里没有对应变更**
// ⇒ 容器一重启，运营在后台配的一切全部丢失。生产实测 llm_routing_rules 至今 0 行，
// 说明这条路径在生产**从未成功落库过**。
//
// 根因：这四个写库函数（Upsert/Delete × provider/route）用的是无 ctx 的裸句柄
// `g := providerDB()`，隔离 callback 拿不到租户绑定 ⇒ fail-closed。
// 同一个文件的**读**路径（LoadProvidersFromDB / LoadRoutesFromDB）走
// `g.WithContext(systemScopeCtx())` —— 同文件、同表、读能过、写不过。
//
// 关键不变量：**隔离闸本身不得被放宽** —— 无租户 ctx 的裸写仍须 fail-closed；
// 放宽的只是这四个「平台级装配写库点」。
func TestProviderAndRouteWrites_OnModeWithoutTenantCtx(t *testing.T) {
	database := testutil.NewTestDB(t, &model.LLMProvider{}, &model.LLMRoutingRule{})
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	// 隔离闸仍在：无租户 ctx 的裸写必须 fail-closed（本修复不得放宽隔离本身）
	if err := database.WithContext(context.Background()).
		Create(&model.LLMProvider{Name: "bare-write-probe", Model: "m"}).Error; err == nil {
		t.Fatal("on 模式无租户 ctx 的裸写必须 fail-closed（隔离不能被本修复放宽）")
	}

	setAuditDB(database)
	defer setAuditDB(nil)

	d := newDispatcherBase(nil)

	// ---- provider 写入 ----
	pc := ProviderConfig{
		Name:         "platform-sensenova",
		DisplayName:  "平台垫资",
		Model:        "deepseek-v4-flash",
		APIType:      "openai",
		Enabled:      true,
		QualityScore: 0.96,
		Source:       "platform",
		PlanCode:     "starter",
	}
	if err := d.UpsertProviderToDB(pc); err != nil {
		t.Fatalf("装配层写 provider 必须在 system scope 下成功: %v", err)
	}

	var gotProvider model.LLMProvider
	if err := database.WithContext(tenant.WithSystemScope(context.Background())).
		Where("name = ?", "platform-sensenova").First(&gotProvider).Error; err != nil {
		t.Fatalf("provider 未落库: %v", err)
	}
	// 归属默认租户：dispatcher 的 LoadProvidersFromDB 只装 tenant_id=1，
	// 写到别的租户上会变成「写进去但读不出来」（provider 永远不可见）。
	if gotProvider.TenantID != tenant.DefaultTenantID {
		t.Fatalf("provider 应归属默认租户 %d，实际 %d", tenant.DefaultTenantID, gotProvider.TenantID)
	}
	if gotProvider.QualityScore != 0.96 {
		t.Fatalf("provider quality_score 未落库: %v", gotProvider.QualityScore)
	}
	if gotProvider.Source != "platform" {
		t.Fatalf("provider source 应为 platform，实际 %q", gotProvider.Source)
	}

	// 重复 upsert 不产生重复行
	if err := d.UpsertProviderToDB(pc); err != nil {
		t.Fatalf("重复 upsert provider 失败: %v", err)
	}
	var cnt int64
	if err := database.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.LLMProvider{}).Where("name = ?", "platform-sensenova").Count(&cnt).Error; err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("重复 upsert 应只有 1 行，实际 %d 行", cnt)
	}

	// ---- 路由写入 ----
	route := ScenarioRoute{
		Scenario:   ScenarioLowCost,
		Provider:   "default",
		Fallbacks:  []string{"platform-sensenova"},
		CostWeight: 15,
		MaxLatency: 720000,
		MinQuality: 0.7,
	}
	if err := d.UpsertRouteToDB(route); err != nil {
		t.Fatalf("装配层写路由必须在 system scope 下成功: %v", err)
	}

	var gotRoute model.LLMRoutingRule
	if err := database.WithContext(tenant.WithSystemScope(context.Background())).
		Where("scenario = ?", string(ScenarioLowCost)).First(&gotRoute).Error; err != nil {
		t.Fatalf("路由未落库: %v", err)
	}
	if gotRoute.TenantID != tenant.DefaultTenantID {
		t.Fatalf("路由应归属默认租户 %d，实际 %d", tenant.DefaultTenantID, gotRoute.TenantID)
	}
	var back ScenarioRoute
	if err := json.Unmarshal([]byte(gotRoute.RouteJSON), &back); err != nil {
		t.Fatalf("路由 JSON 解析失败: %v", err)
	}
	if back.Provider != "default" || len(back.Fallbacks) != 1 || back.Fallbacks[0] != "platform-sensenova" {
		t.Fatalf("路由内容不对: %+v", back)
	}
}

// TestProviderAndRouteDeletes_OnModeWithoutTenantCtx 钉死删除路径：
// 同样走 system scope，否则后台「删除 provider / 路由」删不掉，
// 且内存已删、库还在 ⇒ 重启后又回来了。
func TestProviderAndRouteDeletes_OnModeWithoutTenantCtx(t *testing.T) {
	database := testutil.NewTestDB(t, &model.LLMProvider{}, &model.LLMRoutingRule{})
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	setAuditDB(database)
	defer setAuditDB(nil)

	d := newDispatcherBase(nil)

	pc := ProviderConfig{Name: "to-delete", Model: "m", APIType: "openai", Enabled: true}
	if err := d.UpsertProviderToDB(pc); err != nil {
		t.Fatalf("seed provider 失败: %v", err)
	}
	route := ScenarioRoute{Scenario: ScenarioLowCost, Provider: "default"}
	if err := d.UpsertRouteToDB(route); err != nil {
		t.Fatalf("seed 路由失败: %v", err)
	}

	if err := d.DeleteProviderFromDB("to-delete"); err != nil {
		t.Fatalf("装配层删 provider 必须在 system scope 下成功: %v", err)
	}
	if err := d.DeleteRouteFromDB(ScenarioLowCost); err != nil {
		t.Fatalf("装配层删路由必须在 system scope 下成功: %v", err)
	}

	var pcCount, routeCount int64
	if err := database.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.LLMProvider{}).Where("name = ?", "to-delete").Count(&pcCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.LLMRoutingRule{}).Where("scenario = ?", string(ScenarioLowCost)).Count(&routeCount).Error; err != nil {
		t.Fatal(err)
	}
	if pcCount != 0 {
		t.Fatalf("provider 未被删除，实际剩 %d 行", pcCount)
	}
	if routeCount != 0 {
		t.Fatalf("路由未被删除，实际剩 %d 行", routeCount)
	}
}
