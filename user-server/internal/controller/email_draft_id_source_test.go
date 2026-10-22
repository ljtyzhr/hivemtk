// email_draft_id_source_test.go 邮件草稿更新的 id 取值口径回归。
//
// 背景（任务三逐路由 curl 实测）：`PUT /api/email/drafts/:id` 的控制器
// **只认请求体里的 id**（dto.UpdateEmailDraftRequest.ID），而前端
// `updateDraft(id, data)` 走的是 `PUT /api/email/drafts/${id}` ——
// id 在**路径**上、body 里没有 ⇒ 后端拿到空串 ⇒ 恒 400「无效的 ID 格式」
// ⇒ **邮件草稿的编辑页永远存不进去**。
//
// 同一个资源的 GET / DELETE 读的都是路径参数（uri:"id"），只有 PUT 读 body，
// 三个出口两套口径 —— 这就是漏网的那一个。
//
// 判据（走真实控制器 + 真实库）：
//   - id 只在路径上（前端 updateDraft 的形状）⇒ 200 且草稿真被改到；
//   - 路径与 body 都给且一致 ⇒ 200；
//   - 两者不一致 ⇒ 400（不许悄悄改到别的草稿上）。
package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	pkgdb "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestEmailDraftUpdate_IDFromPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	database := testutil.NewTestDB(t, &model.EmailDraft{})
	if database == nil {
		t.Fatal("测试库不可达：草稿 id 取值口径判不了（不 Skip，跳过等于没锁）")
	}
	prev := pkgdb.GetDB()
	pkgdb.SetTestDB(database)
	t.Cleanup(func() { pkgdb.SetTestDB(prev) })

	// 注意：model.EmailDraft.BeforeCreate 会自己生成 UUID 并覆盖传入的 ID，
	// 所以种子之后要回读一次，拿真正的 id 去拼 URL。
	row := &model.EmailDraft{Subject: "原始标题", Content: "原始正文"}
	if err := database.WithContext(context.Background()).Create(row).Error; err != nil {
		t.Fatalf("seed 草稿失败: %v", err)
	}
	draftID := row.ID.String()

	ctrl := NewEmailDraftController()
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})
	router.PUT("/api/email/drafts/:id", ctrl.UpdateEmailDraft)

	// ① id 只在路径上（前端 updateDraft 的形状）
	body, _ := json.Marshal(map[string]any{"subject": "新标题", "content": "新正文"})
	req, _ := http.NewRequest("PUT", "/api/email/drafts/"+draftID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("id 只在路径上应能更新，实际 %d，body: %s", w.Code, w.Body.String())
	}
	var got model.EmailDraft
	if err := database.WithContext(context.Background()).Where("id = ?", draftID).First(&got).Error; err != nil {
		t.Fatalf("回读草稿失败: %v", err)
	}
	if got.Subject != "新标题" {
		t.Errorf("草稿没被改到: subject=%q want %q", got.Subject, "新标题")
	}

	// ② 路径与 body 同时给且一致 ⇒ 仍 200
	body2, _ := json.Marshal(map[string]any{"id": draftID, "subject": "再改"})
	req2, _ := http.NewRequest("PUT", "/api/email/drafts/"+draftID, bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("路径与 body 一致应能更新，实际 %d，body: %s", w2.Code, w2.Body.String())
	}

	// ③ 两者不一致 ⇒ 400（不许悄悄改到别的草稿上）
	other := uuid.NewString()
	body3, _ := json.Marshal(map[string]any{"id": other, "subject": "串到别处"})
	req3, _ := http.NewRequest("PUT", "/api/email/drafts/"+draftID, bytes.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("路径与 body 不一致应回 400，实际 %d，body: %s", w3.Code, w3.Body.String())
	}
}
