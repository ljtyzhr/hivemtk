// ai_suggestion_controller_test.go AI 建议 HTTP 出口的两条回归：
//  1. 别名路由 `/ai-suggestions?session_id=...` 只带 query，handler 必须回退读 query，
//     否则空串会查成「没有建议」并回 200（e2e deep_customer_sessions.sh 正是这么用的）；
//  2. 标记已使用时行不存在/不属于本租户必须 404，而不是 200「使用成功」。
package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
)

func asgCtrlRouter(t *testing.T) (*gin.Engine, *model.AISuggestion) {
	t.Helper()
	db := testutil.NewTestDB(t, &model.AISuggestion{})
	if db == nil {
		t.Fatal("测试库不可达")
	}

	seed := &model.AISuggestion{
		SessionID:  "sess-1",
		Suggestion: "建议先确认预算",
		Confidence: 88,
		Source:     "llm",
	}
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("预置建议失败: %v", err)
	}

	ctrl := NewAISuggestionController()
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		if uid, err := strconv.ParseUint(ctx.GetHeader("X-Test-Uid"), 10, 32); err == nil && uid != 0 {
			ctx.Set("user_id", uint(uid))
		}
		ctx.Next()
	})
	router.GET("/ai-suggestions", ctrl.GetSuggestions)
	router.GET("/ai-suggestions/:session_id", ctrl.GetSuggestions)
	router.POST("/ai-suggestions/:id/use", ctrl.UseSuggestion)
	return router, seed
}

func asgCtrlDo(router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	req.Header.Set("X-Test-Uid", "901")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func asgCtrlDoAnon(router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestAISuggestionController_QueryParamAliasWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, _ := asgCtrlRouter(t)

	if w := asgCtrlDo(router, http.MethodGet, "/ai-suggestions?session_id=sess-1"); w.Code != http.StatusOK {
		t.Fatalf("query 别名期望 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	if w := asgCtrlDo(router, http.MethodGet, "/ai-suggestions/sess-1"); w.Code != http.StatusOK {
		t.Fatalf("路径参数路由期望 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	if w := asgCtrlDo(router, http.MethodGet, "/ai-suggestions"); w.Code != http.StatusBadRequest {
		t.Fatalf("缺 session_id 期望 400，实际 %d body=%s", w.Code, w.Body.String())
	}
}

func TestAISuggestionController_UseMissingRowReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, seed := asgCtrlRouter(t)

	if w := asgCtrlDo(router, http.MethodPost, "/ai-suggestions/999999/use"); w.Code != http.StatusNotFound {
		t.Fatalf("不存在的建议期望 404，实际 %d body=%s", w.Code, w.Body.String())
	}
	if w := asgCtrlDo(router, http.MethodPost, "/ai-suggestions/"+strconv.FormatUint(uint64(seed.ID), 10)+"/use"); w.Code != http.StatusOK {
		t.Fatalf("本行标记已使用期望 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	if w := asgCtrlDoAnon(router, http.MethodPost, "/ai-suggestions/"+strconv.FormatUint(uint64(seed.ID), 10)+"/use"); w.Code != http.StatusUnauthorized {
		t.Fatalf("无登录身份期望 401（不允许把 used_by 静默写成 0），实际 %d body=%s", w.Code, w.Body.String())
	}
}
