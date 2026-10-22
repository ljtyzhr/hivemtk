package controller

// 契约：POST /api/tiktok-card/generate-short-link 的入参名必须是**前端实际发的那个**。
//
// 坑：前端 `user-web/src/api/tiktokCard.js` 传的是 `{ card_id: cardId }`（snake_case），
// 而 handler 原来绑的是 `json:"cardId"`（camelCase）—— 于是前端这一条**永远 400**
// （binding:"required" 找不到 card_id，请求在门口就被毙掉，压根到不了业务逻辑）。
//
// 兄弟路由（douyin/xiaohongshu/xianyu/kuaishou）把 id 放在**路径**上，不吃这个 body，
// 所以只有 tiktok 这一条踩到了 —— 也正因为如此，光看"路由都在、都能 200"是发现不了的。
//
// 本文件锁两条腿：
//  1. card_id（前端口径）必须能绑上；
//  2. cardId（历史 camelCase 口径）仍要认，别把第三方脚本打断。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func postTikTokShortLink(t *testing.T, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	router := setupTikTokCardTestRouter(t)
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/tiktok-card/generate-short-link", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestTikTokCardGenerateShortLinkAcceptsSnakeCaseCardID 前端口径的 card_id 必须能绑上。
func TestTikTokCardGenerateShortLinkAcceptsSnakeCaseCardID(t *testing.T) {
	w := postTikTokShortLink(t, map[string]any{"card_id": 1})
	if w.Code == http.StatusBadRequest {
		t.Fatalf("card_id 必须能绑上（前端发的就是它），实际 400，body=%s", w.Body.String())
	}
}

// TestTikTokCardGenerateShortLinkStillAcceptsCamelCaseID camelCase 兼容别名不能被这次修复打断。
func TestTikTokCardGenerateShortLinkStillAcceptsCamelCaseID(t *testing.T) {
	w := postTikTokShortLink(t, map[string]any{"cardId": 1})
	if w.Code == http.StatusBadRequest {
		t.Fatalf("cardId 兼容别名必须仍然可用，实际 400，body=%s", w.Body.String())
	}
}
