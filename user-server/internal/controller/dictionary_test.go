package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"

	"hivemtk-user/internal/repository"
)

func newDictCtrl() *DictionaryController {
	// nil service：只验证路由与响应形状，DB 行为在 service 层测。
	return NewDictionaryController(nil)
}

// TestDictionaryRoutesShape 四条路由都注册且响应 JSON 形状符合 API 规范。
func TestDictionaryRoutesShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := newDictCtrl()
	r := gin.New()
	r.GET("/api/manage/dictionaries", ctrl.ListParents)
	r.GET("/api/manage/dictionaries/:parent", ctrl.GetDictionary)
	r.PUT("/api/manage/dictionaries/:parent/items", ctrl.UpsertItem)
	r.PUT("/api/manage/dictionaries/:parent/transitions", ctrl.ReplaceTransitions)

	for _, path := range []string{
		"/api/manage/dictionaries",
		"/api/manage/dictionaries/approval_status",
	} {
		req, _ := http.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s status=%d", path, w.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Errorf("GET %s 非 JSON: %v", path, err)
			continue
		}
		if _, ok := resp["code"]; !ok {
			t.Errorf("GET %s 响应缺 code 字段", path)
		}
		if _, ok := resp["message"]; !ok {
			t.Errorf("GET %s 响应缺 message 字段", path)
		}
	}
}

// TestDictionaryUpsertItemRejectsBadBody 缺 code/name 必须 400，不是 500。
func TestDictionaryUpsertItemRejectsBadBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := newDictCtrl()
	r := gin.New()
	r.PUT("/api/manage/dictionaries/:parent/items", ctrl.UpsertItem)

	req, _ := http.NewRequest("PUT", "/api/manage/dictionaries/x/items",
		httptest.NewRecorder().Result().Body)
	_ = req
	w := httptest.NewRecorder()
	req2, _ := http.NewRequest("PUT", "/api/manage/dictionaries/x/items", nil)
	r.ServeHTTP(w, req2)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(400) {
		t.Errorf("空 body 应 400, got %v (%s)", resp["code"], w.Body.String())
	}
}

// TestDictionaryGetDictionaryUnknownParent 未知域返回 404（不是空列表）：
// 空列表会让管理端以为"这个域就是空的"，从而在下拉里少一个选项且没人发现。
//
// 用真库 fixture：nil service 时 ready() 会先返回 500（那是装配问题，不是域不存在），
// 所以这条必须在 DB 就绪的前提下才验得到 404。
func TestDictionaryGetDictionaryUnknownParent(t *testing.T) {
	database := testutil.NewTestDBOrSkip(t, &model.Dictionary{}, &model.DictionaryTransition{})
	if database == nil {
		t.Skip("no test DB")
	}
	ctrl := NewDictionaryController(service.NewDictionaryService(repository.NewDictionaryRepository(database)))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/manage/dictionaries/:parent", ctrl.GetDictionary)
	req, _ := http.NewRequest("GET", "/api/manage/dictionaries/no_such", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(404) {
		t.Errorf("未知域应 404, got %v (%s)", resp["code"], w.Body.String())
	}
}

// TestDictionaryCtrlNotReadyReturns500 装配顺序出错时返回 500 而不是 panic
// （与 3.2 的 config_param_runtime 同一口径）。
func TestDictionaryCtrlNotReadyReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := newDictCtrl()
	r.GET("/api/manage/dictionaries", ctrl.ListParents)
	r.GET("/api/manage/dictionaries/:parent", ctrl.GetDictionary)
	for _, path := range []string{"/api/manage/dictionaries", "/api/manage/dictionaries/x"} {
		req, _ := http.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var resp map[string]any
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["code"] != float64(500) {
			t.Errorf("GET %s 应 500, got %v", path, resp["code"])
		}
	}
}
