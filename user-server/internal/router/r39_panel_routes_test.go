// r39_panel_routes_test.go R39-K7/K8 面板路由的挂载点与档位。
//
// 与 service 层两份测试的分工：那两份钉"apply/clone 的业务后果"，本份钉
// "这 16 条端点在不在、挂在哪个鉴权组下"。少一条 = 前端照 R39 契约调接口拿 404；
// 多一条 = 有一个没人声明的入口在跑（模板写面是复用资产，不该有旁路）。
package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// r39PanelRouteSpecs 本域交付的全部端点（按字典序写死；判据按集合比）。
// 集合里故意不出现 /api/bots/*：K6 经复算由 channel-agent-bindings +
// channels/overview + 各渠道路由族覆盖，本域不落别名聚合面（C-6 拍板记录）。
var r39PanelRouteSpecs = []string{
	"GET /api/kb-templates",
	"GET /api/kb-templates/:id",
	"POST /api/kb-templates",
	"PUT /api/kb-templates/:id",
	"DELETE /api/kb-templates/:id",
	"POST /api/kb-templates/:id/apply",
	"POST /api/kb-templates/:id/rate",
	"GET /api/dashboard-templates",
	"GET /api/dashboard-templates/:id",
	"POST /api/dashboard-templates",
	"PUT /api/dashboard-templates/:id",
	"DELETE /api/dashboard-templates/:id",
	"POST /api/dashboard-templates/:id/publish",
	"POST /api/dashboard-templates/publish-from-screen",
	"POST /api/dashboard-templates/:id/clone",
	"POST /api/dashboard-templates/:id/rate",
}

func TestR39PanelRoutes_Mounted(t *testing.T) {
	db := testutil.NewTestDB(t, &model.KbTemplate{}, &model.DashboardTemplate{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := r.Group("/api")
	auth.Use(func(c *gin.Context) {
		// 挂载点测试只关心路由表形状，不真过鉴权：塞最小身份让守卫放行。
		c.Set("user_id", uint(1))
		c.Set("role", "admin")
		c.Next()
	})
	setupR39PanelRoutes(auth, db)

	got := map[string]bool{}
	for _, ri := range r.Routes() {
		if strings.HasPrefix(ri.Path, "/api/kb-templates") || strings.HasPrefix(ri.Path, "/api/dashboard-templates") {
			got[ri.Method+" "+ri.Path] = true
		}
	}
	for _, spec := range r39PanelRouteSpecs {
		if !got[spec] {
			t.Errorf("路由缺失：%s", spec)
		}
	}
	for k := range got {
		known := false
		for _, spec := range r39PanelRouteSpecs {
			if k == spec {
				known = true
				break
			}
		}
		if !known {
			t.Errorf("未声明的路由出现：%s", k)
		}
	}
}

// TestR39PanelRoutes_WriteTierGated 模板写面挂租户 admin：无身份直打必须被挡在
// 控制器之外（401/403 都算挡住），不能是控制器回 400——那说明守卫没挂上。
func TestR39PanelRoutes_WriteTierGated(t *testing.T) {
	db := testutil.NewTestDB(t, &model.KbTemplate{}, &model.DashboardTemplate{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := r.Group("/api")
	setupR39PanelRoutes(auth, db)

	for _, tc := range []struct {
		method, path string
	}{{"POST", "/api/kb-templates"}, {"DELETE", "/api/kb-templates/1"}, {"POST", "/api/dashboard-templates"}, {"POST", "/api/dashboard-templates/1/clone"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))
		if w.Code == http.StatusBadRequest {
			t.Errorf("%s %s 未被鉴权挡住（控制器 400 而非守卫 401/403）", tc.method, tc.path)
		}
	}
}
