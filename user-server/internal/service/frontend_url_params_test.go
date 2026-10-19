package service

import (
	"context"
	"os"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// newFrontendURLParamFixture 起一个带 config_params 的库，seed 后换全局参数服务。
func newFrontendURLParamFixture(t *testing.T) *ConfigParamService {
	t.Helper()
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
	if got := getFrontendBaseURL(); got != "https://env.example.com" {
		t.Errorf("env 优先：getFrontendBaseURL() = %q, 期望 https://env.example.com", got)
	}

	// 2) env 清掉后走参数值。
	os.Unsetenv("FRONTEND_URL")
	t.Setenv("FRONTEND_URL", "")
	if got := getFrontendBaseURL(); got != "https://param.example.com" {
		t.Errorf("参数次之：getFrontendBaseURL() = %q, 期望 https://param.example.com", got)
	}

	// 3) 参数未设（seed 默认 http://localhost:3000）时等于代码兜底。
	if err := svc.UpdateValue(ctx, "urls", "frontend_url", "http://localhost:3000", 1); err != nil {
		t.Fatalf("改 urls.frontend_url 回默认失败：%v", err)
	}
	if got := getFrontendBaseURL(); got != "http://localhost:3000" {
		t.Errorf("兜底：getFrontendBaseURL() = %q, 期望 http://localhost:3000", got)
	}
}
