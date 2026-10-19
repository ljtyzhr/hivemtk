package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// 平台交互「租户归属参数」端到端测试（与 platform-server 契约对齐）：
//  1. sync 请求 URL 带 ?tenant_code=<本租户 code>（平台据此下发该租户的订阅段）；
//  2. 订阅续费订单 body 含 tenant_code；
//  3. 取不到 code（无租户行）→ 传空串、照常走通（平台旧行为兜底）。

// tenantCodeCaptures 捕获 sync query 与下单 body 的桩平台。
//
// 经带缓冲 channel 回传（对齐 client_test.go refusalServer 的理由）：handler
// 跑在服务器 goroutine 上，共享变量会被 -race 判竞态。
type tenantCodeCaptures struct {
	syncQueryCh chan string
	orderBodyCh chan []byte
}

func setupTenantCodeStub(t *testing.T) *tenantCodeCaptures {
	t.Helper()
	cap := &tenantCodeCaptures{
		syncQueryCh: make(chan string, 8),
		orderBodyCh: make(chan []byte, 8),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/merchant-api/modules/sync":
			select {
			case cap.syncQueryCh <- r.URL.RawQuery:
			default:
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "", "data": map[string]any{
				"merchant_key": "MKE2E",
				"active_keys":  []string{"core"},
				"menu_keys":    []string{},
				"catalog": []map[string]any{
					{"module_key": "core", "name": "core", "category": "base",
						"owned": true, "owned_status": "active", "owned_source": "default_base", "menu_keys": []string{}},
				},
			}})
		case "/merchant-api/orders":
			// 续费下单（POST）与订单状态列表（GET）同路径：按方法分流
			if r.Method == http.MethodPost {
				body, _ := io.ReadAll(r.Body)
				select {
				case cap.orderBodyCh <- body:
				default:
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "", "data": map[string]any{
					"order_no": "SUB-TENANT-CODE-1", "status": "pending_payment",
					"total_amount_cents": 14900, "items": []any{},
					"payment_guide": "联系运营付款",
				}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "", "data": map[string]any{
				"total": 0, "list": []map[string]any{},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	prev := config.PlatformCfg
	config.PlatformCfg = &config.PlatformConfig{APIURL: srv.URL, Secret: "test-secret"}
	t.Cleanup(func() { config.PlatformCfg = prev })
	return cap
}

func setupTenantCodeDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.Tenant{}, &model.TenantModule{}, &model.ModuleSyncLog{}, &model.ModuleOrder{})
	db.SetTestDB(database)
	// on 模式下无绑定删除会被 fail-closed：清空挂 SystemScope（cleanup 跑时 mode 仍是 on，
	// 顺序约定与 clearModuleTablesOnMode 相同：先 withTenantMode 再本函数）
	t.Cleanup(func() {
		sysCtx := tenant.WithSystemScope(context.Background())
		for _, m := range []any{&model.Tenant{}, &model.TenantModule{}, &model.ModuleSyncLog{}, &model.ModuleOrder{}} {
			if err := database.Session(&gorm.Session{AllowGlobalUpdate: true}).
				WithContext(sysCtx).Delete(m).Error; err != nil {
				t.Logf("清理 %T 失败: %v", m, err)
			}
		}
	})
	return database
}

// TestModuleSyncSendsTenantCodeQuery on 模式：sync 请求必须带本租户 code。
func TestModuleSyncSendsTenantCodeQuery(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	t.Setenv("PLATFORM_ENABLED", "true")
	t.Setenv("PLATFORM_MERCHANT_KEY", "MKE2E")

	database := setupTenantCodeDB(t)
	stub := setupTenantCodeStub(t)

	tn := &model.Tenant{Code: "tCODE01", Name: "带码租户"}
	if err := database.Create(tn).Error; err != nil {
		t.Fatalf("写入租户行: %v", err)
	}

	// 请求 ctx 绑定该租户（与 TenantMiddleware → 手动同步按钮同形）；
	// Sync 内部挂 SystemScope 后仍应解析出绑定租户的 code。
	ctx := tenant.WithTenantID(context.Background(), tn.ID)
	svc := NewModuleCatalogService()
	if _, err := svc.Sync(ctx); err != nil {
		t.Fatalf("Sync 失败: %v", err)
	}
	if got := <-stub.syncQueryCh; got != "tenant_code=tCODE01" {
		t.Errorf("sync URL 应带 ?tenant_code=tCODE01，实际 %q", got)
	}
}

// TestModuleSyncEmptyTenantCodeWhenNoTenantRow 租户表无对应行 → 传空串照发
// （平台旧行为兜底），不许报错。
func TestModuleSyncEmptyTenantCodeWhenNoTenantRow(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	t.Setenv("PLATFORM_ENABLED", "true")
	t.Setenv("PLATFORM_MERCHANT_KEY", "MKE2E")

	setupTenantCodeDB(t)
	stub := setupTenantCodeStub(t)

	// 绑定一个不存在的租户 id：查不到行 → code 空串
	ctx := tenant.WithTenantID(context.Background(), 4242)
	svc := NewModuleCatalogService()
	if _, err := svc.Sync(ctx); err != nil {
		t.Fatalf("查不到租户行时 Sync 不应报错: %v", err)
	}
	if got := <-stub.syncQueryCh; got != "tenant_code=" {
		t.Errorf("查不到租户行应发 ?tenant_code=，实际 %q", got)
	}
}

// TestSubscriptionRenewCarriesTenantCode 续费下单 body 必须含本租户 code。
func TestSubscriptionRenewCarriesTenantCode(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	t.Setenv("PLATFORM_ENABLED", "true")
	t.Setenv("PLATFORM_MERCHANT_KEY", "MKE2E")

	database := setupTenantCodeDB(t)
	stub := setupTenantCodeStub(t)

	tn := &model.Tenant{Code: "tRENEW1", Name: "续费租户"}
	if err := database.Create(tn).Error; err != nil {
		t.Fatalf("写入租户行: %v", err)
	}

	ctx := tenant.WithTenantID(context.Background(), tn.ID)
	res, err := NewSubscriptionMirrorService().Renew(ctx, "CLI-SUB-TC-1", "张三", "13800000000", "", "")
	if err != nil {
		t.Fatalf("Renew 失败: %v", err)
	}
	if res.OrderNo != "SUB-TENANT-CODE-1" {
		t.Fatalf("续费未走通平台下单: %+v", res)
	}
	var body struct {
		TenantCode string `json:"tenant_code"`
		OrderType  string `json:"order_type"`
	}
	raw := <-stub.orderBodyCh
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("解析续费下单 body: %v (%s)", err, raw)
	}
	if body.TenantCode != "tRENEW1" {
		t.Errorf("续费下单 body tenant_code 应为 tRENEW1，实际 %q", body.TenantCode)
	}
	if body.OrderType != "subscription" {
		t.Errorf("order_type 应为 subscription，实际 %q", body.OrderType)
	}
}
