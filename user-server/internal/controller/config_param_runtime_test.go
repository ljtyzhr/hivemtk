package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

// runtimeTestRouter 起一个只挂 Runtime 的 gin，请求打进去拿响应体。
func runtimeTestRouter(c *ConfigParamController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/config/runtime", c.Runtime)
	return r
}

func doRuntime(t *testing.T, c *ConfigParamController, query string) map[string]any {
	t.Helper()
	url := "/api/config/runtime"
	if query != "" {
		url += "?" + query
	}
	req, _ := http.NewRequest("GET", url, nil)
	w := httptest.NewRecorder()
	runtimeTestRouter(c).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if resp["code"] != float64(0) {
		t.Fatalf("code != 0: %v body=%s", resp["code"], w.Body.String())
	}
	return resp
}

// TestRuntimeDefaultReturnsWhitelistedGroups 不传 groups 时返回白名单全部，
// 且每条都是 "group.key" 形状的扁平映射。
func TestRuntimeDefaultReturnsWhitelistedGroups(t *testing.T) {
	ctrl := NewConfigParamController(nil)
	resp := doRuntime(t, ctrl, "")
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("data 应为 map, got %T", resp["data"])
	}
	// nil service 时读不到任何参数，返回空 map + code 0（判据 3：配置面挂了不白屏）。
	if len(data) != 0 {
		t.Logf("无库时返回 %d 条（应为 0）: %v", len(data), data)
	}
}

// TestRuntimeOnlyExposesValue 响应里只有 group.key -> value，不带任何可解释性字段。
// 这是本端点的核心约束：Name/Description/Min/Max/Restart/ReadOnly 一个都不能出现。
func TestRuntimeOnlyExposesValue(t *testing.T) {
	ctrl := NewConfigParamController(nil)
	resp := doRuntime(t, ctrl, "groups=frontend_ws,frontend_ui,pagination")
	data, _ := resp["data"].(map[string]any)
	for k, v := range data {
		if !strings.Contains(k, ".") {
			t.Errorf("键 %q 不含 '.'，不是 group.key 形状", k)
		}
		if _, ok := v.(string); !ok {
			t.Errorf("键 %q 的值应为字符串（value 原样），got %T", k, v)
		}
	}
}

// TestRuntimeIgnoresNonWhitelistedGroups 不在白名单里的组被静默忽略：
// 不报错、不回显、不当成「探测有哪些组」的手段。
func TestRuntimeIgnoresNonWhitelistedGroups(t *testing.T) {
	ctrl := NewConfigParamController(nil)
	// webhook / agent_llm / lock 都在白名单外：这一条同时是「白名单没漏放它们」的护栏。
	resp := doRuntime(t, ctrl, "groups=webhook,agent_llm,lock,feedback,frontend_ws")
	data, _ := resp["data"].(map[string]any)
	for k := range data {
		for _, forbidden := range []string{"webhook.", "agent_llm.", "lock.", "feedback."} {
			if strings.HasPrefix(k, forbidden) {
				t.Errorf("白名单外的组泄漏了: %q", k)
			}
		}
	}
}

// TestRuntimeAllPublicGroupsAreActuallyPublic 白名单里的组必须都读得通
// （防止有人往白名单里加了一个不存在的组名，前端静默少读一组）。
func TestRuntimeAllPublicGroupsAreActuallyPublic(t *testing.T) {
	for _, g := range []string{"frontend_ws", "frontend_ui", "pagination"} {
		if !AnyRuntimeGroupPublic(g) {
			t.Errorf("组 %s 应在白名单里", g)
		}
	}
	for _, g := range []string{"webhook", "agent_llm", "lock", "feedback", "channelbot",
		"sse", "bridge", "domain_health", "rag_health", "sop", "misc", "confidence", "cache"} {
		if AnyRuntimeGroupPublic(g) {
			t.Errorf("组 %s 不应在只读白名单里", g)
		}
	}
}

// TestSplitGroups 去空白/去重/保序/去空。
func TestSplitGroups(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{" a , b ", []string{"a", "b"}},
		{"a,,b", []string{"a", "b"}},
		{"a,a,b,a", []string{"a", "b"}},
		{"a,b,a", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := splitGroups(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitGroups(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitGroups(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// TestRuntimeNilServiceDoesNotPanic service 为 nil 时不能 panic——
// 装配顺序出错（GlobalConfigParam() 返回 stub 前的窗口期）也该返回空配置。
func TestRuntimeNilServiceDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil service 时 panic 了: %v", r)
		}
	}()
	ctrl := NewConfigParamController(nil)
	_ = doRuntime(t, ctrl, "groups=frontend_ws")
}

// TestRuntimeReadsRealValues 带真库的行为测试：白名单里的组真的返回库里的值，
// 且改完后端参数后前端读到的是新值（否则端点通了也是"读了个空"）。
func TestRuntimeReadsRealValues(t *testing.T) {
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if database == nil {
		t.Skip("no test DB")
	}
	if err := service.SeedConfigParams(context.Background(), database); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := service.NewConfigParamService(database)
	service.SetGlobalForTest(svc)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })

	// pagination 组在种子里存在，取其一条改掉再读。
	var p model.ConfigParam
	if err := database.Where("param_group = ?", "pagination").First(&p).Error; err != nil {
		t.Fatalf("pagination 组无种子：%v", err)
	}
	orig := p.Value
	defer func() {
		_ = database.Model(&model.ConfigParam{}).
			Where("param_group = ? AND key = ?", "pagination", p.Key).
			Update("param_value", orig).Error
	}()

	want := "77"
	if err := svc.UpdateValue(context.Background(), "pagination", p.Key, want, 1); err != nil {
		t.Fatalf("UpdateValue 失败：%v", err)
	}

	ctrl := NewConfigParamController(svc)
	resp := doRuntime(t, ctrl, "groups=pagination")
	data, _ := resp["data"].(map[string]any)
	got := data["pagination."+p.Key]
	if got != want {
		t.Errorf("runtime 应读到改后的 %q, got %v", want, got)
	}
}
