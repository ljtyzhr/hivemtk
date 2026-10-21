// ai_tool_config_ctrl_test.go AI 工具状态出口的回归：
// is_enabled 漏传必须 400，不能被 bool 零值当成「关掉工具」静默落库。
package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

func atcCtrlRouter(t *testing.T) (*gin.Engine, *model.AIToolConfig) {
	t.Helper()
	db := testutil.NewTestDB(t, &model.AIToolConfig{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	seed := &model.AIToolConfig{ToolName: "reach.tool", Category: "reach", IsEnabled: true}
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("预置工具失败: %v", err)
	}

	svc := service.NewAIToolConfigService(
		repository.NewAIToolConfigRepository(db),
		repository.NewAIToolAccountBindingRepository(db),
	)
	ctrl := NewAIToolConfigController(svc)

	router := gin.New()
	router.PUT("/ai-tools/:name/status", ctrl.UpdateToolStatus)
	router.POST("/ai-tools/batch-status", ctrl.BatchUpdateStatus)
	return router, seed
}

func atcCtrlDo(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestAIToolConfigController_StatusRequiresExplicitFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, _ := atcCtrlRouter(t)

	if w := atcCtrlDo(router, http.MethodPut, "/ai-tools/reach.tool/status", `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("漏传 is_enabled 期望 400，实际 %d body=%s", w.Code, w.Body.String())
	}
	if w := atcCtrlDo(router, http.MethodPost, "/ai-tools/batch-status", `{"tools":["reach.tool"]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("批量漏传 is_enabled 期望 400，实际 %d body=%s", w.Code, w.Body.String())
	}
}
