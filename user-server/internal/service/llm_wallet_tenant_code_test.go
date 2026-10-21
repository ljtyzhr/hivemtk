package service

import (
	"context"
	"encoding/json"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
)

// LLM 套餐下单「租户归属参数」测试（与 platform-server 契约对齐）：
// POST /merchant-api/orders 的 body 必须含 tenant_code —— 平台端 LLM 订单
// 按 (merchant_key, tenant_code) 归属，入的是**本租户**的钱包。
//
// 桩平台 / DB 装配复用 module_tenant_code_test.go（同包）的
// setupTenantCodeStub / setupTenantCodeDB。

// TestLLMOrderCarriesTenantCode LLM 套餐下单 body 必须含本租户 code。
func TestLLMOrderCarriesTenantCode(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	t.Setenv("PLATFORM_ENABLED", "true")
	t.Setenv("PLATFORM_MERCHANT_KEY", "MKE2E")

	database := setupTenantCodeDB(t)
	stub := setupTenantCodeStub(t)

	tn := &model.Tenant{Code: "tLLM001", Name: "LLM 租户"}
	if err := database.Create(tn).Error; err != nil {
		t.Fatalf("写入租户行: %v", err)
	}

	ctx := tenant.WithTenantID(context.Background(), tn.ID)
	res, err := NewLLMWalletService().CreateOrder(ctx, LLMOrderInput{
		PlanCode:      "basic",
		ClientOrderNo: "CLI-LLM-TC-1",
		ContactPhone:  "13800000000",
	})
	if err != nil {
		t.Fatalf("CreateOrder 失败: %v", err)
	}
	if res.OrderNo == "" {
		t.Fatalf("下单未走通平台: %+v", res)
	}
	// 在线支付参数必须透传：平台已下发 payment 段时前端要直接开收银台，
	// 不能再回落"联系运营/加微信"的线下指引。
	if res.Payment == nil || res.Payment.PayURL == "" {
		t.Fatalf("LLM 下单必须透传平台 payment 段，实际 %+v", res.Payment)
	}

	var body struct {
		TenantCode    string `json:"tenant_code"`
		OrderType     string `json:"order_type"`
		PlanCode      string `json:"plan_code"`
		ClientOrderNo string `json:"client_order_no"`
	}
	raw := <-stub.orderBodyCh
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("解析 LLM 下单 body: %v (%s)", err, raw)
	}
	if body.TenantCode != "tLLM001" {
		t.Errorf("LLM 下单 body tenant_code 应为 tLLM001，实际 %q", body.TenantCode)
	}
	if body.OrderType != "llm" {
		t.Errorf("order_type 应为 llm，实际 %q", body.OrderType)
	}
	if body.PlanCode != "basic" || body.ClientOrderNo != "CLI-LLM-TC-1" {
		t.Errorf("既有下单字段被改动: %+v", body)
	}
}

// TestLLMOrderEmptyTenantCodeStillCalls 租户表无对应行 → code 空串、
// omitempty 不上行（平台旧行为兜底），下单照发不许报错。
func TestLLMOrderEmptyTenantCodeStillCalls(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	t.Setenv("PLATFORM_ENABLED", "true")
	t.Setenv("PLATFORM_MERCHANT_KEY", "MKE2E")

	setupTenantCodeDB(t)
	stub := setupTenantCodeStub(t)

	ctx := tenant.WithTenantID(context.Background(), 4242)
	if _, err := NewLLMWalletService().CreateOrder(ctx, LLMOrderInput{
		PlanCode:     "basic",
		ContactPhone: "13800000000",
	}); err != nil {
		t.Fatalf("查不到租户行时 CreateOrder 不应报错: %v", err)
	}

	raw := <-stub.orderBodyCh
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("解析 LLM 下单 body: %v (%s)", err, raw)
	}
	if _, exists := body["tenant_code"]; exists {
		t.Errorf("空 tenant_code 应被 omitempty 省略，实际 body=%s", raw)
	}
	if body["order_type"] != "llm" {
		t.Errorf("order_type 应为 llm，实际 %v", body["order_type"])
	}
}
