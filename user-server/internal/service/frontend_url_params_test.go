package service

import (
	"context"
	"os"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// newFrontendURLParamFixture 起一个带 config_params 的库，seed 后换全局参数服务。
func newFrontendURLParamFixture(t *testing.T) *ConfigParamService {
	t.Helper()
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if database == nil {
		t.Skip("no test DB")
	}
	if err := SeedConfigParams(context.Background()); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := NewConfigParamService()
	SetGlobalForTest(svc)
	t.Cleanup(func() { SetGlobalForTest(nil) })
	return svc
}

// TestGetFrontendBaseURL_TieredPriority 三级优先：env > 参数 > 代码兜底。
func TestGetFrontendBaseURL_TieredPriority(t *testing.T) {
	svc := newFrontendURLParamFixture(t)
	ctx := context.Background()

	// 1) env 优先：即便参数库里有值，env 一设就压过它。
	t.Setenv("FRONTEND_URL", "https://env.example.com")
	if err := svc.UpdateValue(ctx, "urls", "frontend_url", "https://param.example.com", 1); err != nil {
		t.Fatalf("改 urls.frontend_url 失败：%v", err)
	}
	if got := getFrontendBaseURL(ctx); got != "https://env.example.com" {
		t.Errorf("env 优先：getFrontendBaseURL(ctx) = %q, 期望 https://env.example.com", got)
	}

	// 2) env 清掉后走参数值。
	os.Unsetenv("FRONTEND_URL")
	t.Setenv("FRONTEND_URL", "")
	if got := getFrontendBaseURL(ctx); got != "https://param.example.com" {
		t.Errorf("参数次之：getFrontendBaseURL(ctx) = %q, 期望 https://param.example.com", got)
	}

	// 3) 参数未设（seed 默认 http://localhost:3000）时等于代码兜底。
	if err := svc.UpdateValue(ctx, "urls", "frontend_url", "http://localhost:3000", 1); err != nil {
		t.Fatalf("改 urls.frontend_url 回默认失败：%v", err)
	}
	if got := getFrontendBaseURL(ctx); got != "http://localhost:3000" {
		t.Errorf("兜底：getFrontendBaseURL(ctx) = %q, 期望 http://localhost:3000", got)
	}
}

// TestGetFrontendBaseURLTenantOverride R5-3：重置邮件的前端基址必须按**发信对象所属租户**
// 解析。用 context.Background() 拍平过一次，A 配的自有域名就会发出 B 的链接。
func TestGetFrontendBaseURLTenantOverride(t *testing.T) {
	database := testutil.NewTestDBOrSkip(t,
		&model.ConfigParam{}, &model.ConfigParamAuditLog{},
		&model.TenantConfigParam{}, &model.TenantConfigParamAuditLog{},
	)
	if database == nil {
		t.Skip("no test DB")
	}
	if err := SeedConfigParams(context.Background()); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := NewConfigParamService()
	SetGlobalForTest(svc)
	t.Cleanup(func() { SetGlobalForTest(nil) })
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	tenant.SetMode(tenant.ModeOn)

	t.Setenv("FRONTEND_URL", "")
	ctxA := tenant.WithTenantID(context.Background(), 91)
	ctxB := tenant.WithTenantID(context.Background(), 92)
	if err := svc.UpdateValue(ctxA, "urls", "frontend_url", "https://a.example.com", 1); err != nil {
		t.Fatalf("A 覆盖 urls.frontend_url 失败：%v", err)
	}
	if got := getFrontendBaseURL(ctxA); got != "https://a.example.com" {
		t.Errorf("A 的重置链接基址 = %q, 期望自己的覆盖值 https://a.example.com", got)
	}
	// B 未覆盖 ⇒ 回落平台默认（seed 值），绝不能拿到 A 的域名
	if got := getFrontendBaseURL(ctxB); got != "http://localhost:3000" {
		t.Errorf("B 不得读到 A 的覆盖值，期望平台默认 http://localhost:3000，实际 %q", got)
	}
}
