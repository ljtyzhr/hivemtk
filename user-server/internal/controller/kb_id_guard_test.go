// kb_id_guard_test.go P0-3：全局 List 端点不许静默忽略 kb_id。
//
// 缺陷形状（修复前）：/api/faqs 与 /api/sop-templates 的 List 只读
// keyword/category/intent/enabled/stage + 分页，kb_id 被丢掉 ⇒ 前端按知识库
// 预览时拿到的是**全局列表**：不报错、不空列表，只是内容不是那个知识库的
// ——比 404 更隐蔽的错数据。
//
// 判据（两个端点各一对）：
//   - 带 kb_id ⇒ 400，且响应里点名 kb_id（"拒绝"必须可归因，不能是别的 400）；
//   - 不带 kb_id（以及 kb_id= 空串）⇒ 不因此被拒（正控制：守卫只对真传值开火，
//     否则这条 400 会把正常列表也一起打死，测试仍绿）。
package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// kbGuardResponse 只取响应里判据要用的两格：HTTP 码判拒绝，message/code 判归因。
type kbGuardResponse struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
}

// runKBGuardCase 在真 gin 引擎上打一次 GET（不经全局路由：判据是 handler 自己的
// 守卫，挂到路由表上会让"路由没注册"与"守卫没开火"两种红长得一样）。
func runKBGuardCase(t *testing.T, register func(*gin.RouterGroup), path string) (int, kbGuardResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Recovery：正控制腿会走到 service（本用例不装 DB），那边以 panic/500 形态收场；
	// 判据只看"是不是 kb_id 那条 400"，所以让失败以响应形式出现而不是掀翻测试进程。
	r.Use(gin.Recovery())
	register(r.Group("/api"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	var body kbGuardResponse
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

func TestListEndpointsRejectKBIDInsteadOfIgnoringIt(t *testing.T) {
	faq := (&FAQController{}).RegisterRoutes
	sop := (&SOPTemplateController{}).RegisterRoutes

	for _, tc := range []struct {
		name string
		reg  func(*gin.RouterGroup)
	}{
		{"faqs", faq},
		{"sop-templates", sop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 判据：带 kb_id ⇒ 400 且消息点名 kb_id（拒绝可归因）。
			code, body := runKBGuardCase(t, tc.reg, "/api/"+tc.name+"?kb_id=kb_abc&page=1&page_size=20")
			if code != http.StatusBadRequest {
				t.Fatalf("带 kb_id 的列表请求回了 %d，期望 400（静默忽略 = 显示错数据）", code)
			}
			if !strings.Contains(body.Message, "kb_id") {
				t.Fatalf("400 的响应没有点名 kb_id，无法归因：%q", body.Message)
			}
			// 同一句里必须指路，否则调用方只知道"被拒"，不知道改调哪里。
			if !strings.Contains(body.Message, "/api/knowledge-bases/") {
				t.Fatalf("拒绝消息没有指路（按知识库预览应指向 /api/knowledge-bases/:id/items）：%q", body.Message)
			}

			// 正控制 1：kb_id 只有空白 ⇒ 视同未传，守卫不许开火。
			if code, body := runKBGuardCase(t, tc.reg, "/api/"+tc.name+"?kb_id=%20%20&page=1"); code == http.StatusBadRequest && strings.Contains(body.Message, "kb_id 过滤不支持") {
				t.Fatalf("空白 kb_id 触发了守卫（它等于没传）：%d %q", code, body.Message)
			}
			// 正控制 2：不传 kb_id ⇒ 不进守卫分支（本用例没有 DB，走到 service 会
			// 以非 400 的失败形态出现；判据只要求"不是 kb_id 那条 400"）。
			if code, body := runKBGuardCase(t, tc.reg, "/api/"+tc.name+"?page=1"); code == http.StatusBadRequest && strings.Contains(body.Message, "kb_id 过滤不支持") {
				t.Fatalf("没传 kb_id 也被守卫拒了：%d %q", code, body.Message)
			}
		})
	}
}
