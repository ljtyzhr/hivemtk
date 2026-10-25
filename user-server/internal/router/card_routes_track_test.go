package router

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TestXianyuCardTrackRoutes_RegisteredOnPublicGroup 验证三条埋点写口真的注册成了
// POST /api/xianyu/:id/{click,share,view}，并且与 auth 档里已有的
// /xianyu/create、/xianyu/:id/generate-short-link 等静态/参数路由共存不冲突
// （gin 的冲突会在注册期直接 panic，本用例不需要连库就能挡住）。
//
// 鉴权不靠路由组：组上故意不挂任何身份中间件（页面无登录态），
// 凭证校验在 handler 内，行为腿见 controller/xianyu_card_track_test.go。
func TestXianyuCardTrackRoutes_RegisteredOnPublicGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	database := &gorm.DB{}
	setupCardRoutes(engine.Group("/api"), database)
	setupXianyuCardTrackRoutes(engine.Group("/api"), database)

	want := map[string]string{
		"POST /api/xianyu/:id/click": "RecordClick",
		"POST /api/xianyu/:id/share": "RecordShare",
		"POST /api/xianyu/:id/view":  "RecordView",
	}
	got := make(map[string]string, len(engine.Routes()))
	for _, ri := range engine.Routes() {
		got[ri.Method+" "+ri.Path] = ri.Handler
	}
	for key, handler := range want {
		name, ok := got[key]
		if !ok {
			t.Errorf("埋点写口没有注册：%s（现有路由 %d 条）", key, len(got))
			continue
		}
		if !strings.Contains(name, handler) {
			t.Errorf("%s 应指向 XianyuCardController.%s，实际 terminal handler=%s", key, handler, name)
		}
	}
}
