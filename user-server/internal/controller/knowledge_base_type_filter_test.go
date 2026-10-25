// knowledge_base_type_filter_test.go 把 GET /api/knowledge-bases 的**类型过滤**
// 钉在 HTTP 层。
//
// 缺陷背景（任务四 UI 拟人测试实测）：前端 user-web/src/views/knowledgeBase/List.vue
// 一直发 `?kb_type=faq`，而后端读的是 `?type=`（controller/knowledge_base.go 的
// @Param type 也是这个口径）⇒ 类型过滤**从未生效**。表现是页面上 RAG / FAQ / SOP
// 三个标签页拿到的都是同一份全量列表：切标签看不到任何变化，用户只会以为
// "库里就这些"。这跟"库里真的只有这些"无法区分，必须用两个不同类型的库来断言。
//
// 修法：后端同时接受 type（文档口径）与 kb_type（兼容在跑的旧前端）；
// 前端改回发 type。
package controller

import (
	"context"
	"net/http"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// 造两个不同类型的知识库，返回一个带它们的 router。
func newKBTypeFilterRouter(t *testing.T) (*gin.Engine, uint, uint) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	testutil.NewTestDB(t, &model.KnowledgeBase{})
	ctrl := &KnowledgeBaseController{svc: service.NewKnowledgeBaseService()}
	router := gin.New()
	ctrl.RegisterRoutes(router.Group("/api"))

	enabled := true
	faq := &model.KnowledgeBase{
		KBCode: "kb-type-filter-faq", Type: model.KnowledgeBaseTypeFAQ, Name: "过滤用 FAQ 库",
		OwnerType: model.KnowledgeBaseOwnerShared, Enabled: &enabled,
	}
	rag := &model.KnowledgeBase{
		KBCode: "kb-type-filter-rag", Type: model.KnowledgeBaseTypeRAG, Name: "过滤用 RAG 库",
		OwnerType: model.KnowledgeBaseOwnerShared, Enabled: &enabled,
	}
	if err := ctrl.svc.CreateKB(context.Background(), faq); err != nil {
		t.Fatalf("建 FAQ 库失败: %v", err)
	}
	if err := ctrl.svc.CreateKB(context.Background(), rag); err != nil {
		t.Fatalf("建 RAG 库失败: %v", err)
	}
	return router, faq.ID, rag.ID
}

// 列表返回的每一条都要是请求的那个类型；且必须只回一条（库里总共两个库，
// 过滤不生效就会回两条）。
func assertListOnlyType(t *testing.T, router *gin.Engine, query, wantType string) {
	t.Helper()
	code, _, data := kbHandlerCall(t, router, http.MethodGet, "/api/knowledge-bases"+query, "")
	if code != http.StatusOK {
		t.Fatalf("%s: 期望 200，实际 %d", query, code)
	}
	raw, ok := data["list"].([]any)
	if !ok {
		t.Fatalf("%s: 响应里没有 list 数组: %v", query, data)
	}
	if len(raw) != 1 {
		t.Fatalf("%s: 期望只回 1 条（过滤生效），实际 %d 条 —— 类型过滤没生效", query, len(raw))
	}
	row, ok := raw[0].(map[string]any)
	if !ok {
		t.Fatalf("%s: list 元素不是对象: %v", query, raw[0])
	}
	if got, _ := row["type"].(string); got != wantType {
		t.Errorf("%s: 期望 type=%q，实际 %q", query, wantType, got)
	}
}

// 文档口径 ?type=
func TestKBList_TypeParamFilters(t *testing.T) {
	router, _, _ := newKBTypeFilterRouter(t)
	assertListOnlyType(t, router, "?type=faq", "faq")
	assertListOnlyType(t, router, "?type=rag", "rag")
}

// 兼容口径 ?kb_type=（旧前端发的参数名）
func TestKBList_KBTypeParamAlsoFilters(t *testing.T) {
	router, _, _ := newKBTypeFilterRouter(t)
	assertListOnlyType(t, router, "?kb_type=faq", "faq")
	assertListOnlyType(t, router, "?kb_type=rag", "rag")
}

// 不带类型参数时回全量（两个库）—— 过滤生效不等于把不分类型的用法也堵死。
func TestKBList_NoTypeParamReturnsAll(t *testing.T) {
	router, _, _ := newKBTypeFilterRouter(t)
	code, _, data := kbHandlerCall(t, router, http.MethodGet, "/api/knowledge-bases", "")
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", code)
	}
	raw, _ := data["list"].([]any)
	if len(raw) != 2 {
		t.Errorf("不带类型参数时应回全量 2 条，实际 %d 条", len(raw))
	}
}
